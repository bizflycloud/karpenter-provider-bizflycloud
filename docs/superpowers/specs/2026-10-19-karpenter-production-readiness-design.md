# Karpenter BizflyCloud — Production Readiness Design

**Date**: 2026-10-19  
**Status**: Approved by user  
**Repos**: `karpenter-provider-bizflycloud-2`, `bizflycloud-kubernetes-add-ons`, `bke-orchestrator`

---

## 1. Problem Statement

Karpenter BizflyCloud code đã được fix (session 2026-10-18) nhưng chưa có pipeline
từ code → production. Mục tiêu: cài được qua BKE add-on UI trên production cluster.

---

## 2. Current State

### 2.1 Release Pipeline (As-Is)
```
Dev → git tag vX.Y.Z → GitHub Actions → Docker push
→ ❌ MANUAL: update addon repo (values.yml, INFORMATION.yaml, Chart.yaml)
→ bke-addons:v1.0.6 runs deploy.sh → kubectl apply CRDs → helm install
```

**Gaps:**
- Không có automated version bump từ provider release → addon repo
- Không có test gate trước khi release
- Dual-copy helm charts (provider repo vs addon repo) diverged

### 2.2 Add-on Installation Flow (As-Is)
```
[User enables Karpenter add-on]
  → bke-orchestrator/cluster_updater.py (L523-590)
      → Create Secret 'bizflycloud-credentials' (13 keys) trực tiếp vào shoot cluster
      → Spawn Job: bke-addons:v1.0.6 trên seed cluster
          → deploy.sh → kubectl apply crds/ → helm install ./helm
```

**Key insight**: `secret.yaml` trong helm là dead code — orchestrator tạo secret trực tiếp.
Helm `bizflycloud.appCredId` không bao giờ được pass → secret.yaml không render.

### 2.3 Blast Radius Assessment

| Rủi ro | Mức | Lý do |
|--------|-----|-------|
| Existing nodes bị xóa | 🟢 Zero | Karpenter chỉ tạo node mới |
| Cluster workload gián đoạn | 🟢 Zero | Không có webhook; namespace isolation |
| Autoscaling blocked khi rollout | 🟡 ~30s | `Recreate` strategy |
| Node xóa sai do consolidation | 🟡 Kiểm soát | budget `1 node`, `consolidateAfter: 30s` |
| HCM cluster sai zone | 🟡 Possible | `values.yml` default `region: HN` |

**Kết luận**: Blast radius THẤP — an toàn để controlled rollout.

---

## 3. Gaps To Fix (Priority Order)

### HIGH (blocking usable)

**G1: `default-nodeclass.yaml` thiếu `nodeCategories`**
- File: `bizflycloud-kubernetes-add-ons/karpenter/helm/templates/default-nodeclass.yaml`
- Problem: NodeClass không có `nodeCategories` field → Karpenter provision node sai tier
- Fix: Thêm `nodeCategories` từ `.Values.nodeClass.nodeCategories` (default: `[]` = all categories)

**G6: Không có GitLab CI cho karpenter provider trong BizFly internal GitLab**
- Problem: GitHub Actions only → không tích hợp với BizFly GitLab CI/CD workflow
- Fix: Thêm `.gitlab-ci.yml` với stages: `build`, `push`, `update-addon`

**G7: Zero integration/e2e tests**
- Problem: `test/integration/`, `test/e2e/` đều empty → không có confidence khi release
- Fix: Smoke test cơ bản (NodeClaim creation) + scale test

### MEDIUM

**G9: `matrix_addons.json` thiếu karpenter entry**
- File: `bke-orchestrator/app/data/matrix_addons.json`
- Problem: Không có version compatibility mapping per K8s version
- Fix: Thêm karpenter version mapping

**G13: `values.yml` HCM region default**
- Problem: `region: HN`, `zones: [HN1, HN2]` hardcoded default
- Fix: Orchestrator đã override via `--set`, nhưng cần verify flow cho HCM clusters

**G8: Dual-copy helm charts diverged**
- Provider repo `bke-addon/helm/` (version 0.32.0) ≠ Addon repo helm (version 1.2.2-beta.1)
- Fix: Xóa/archive `bke-addon/helm/` trong provider repo, reference only addon repo

### LOW

**G2: `secret.yaml` key `logEndpoint` → `log-endpoint`**
- Không ảnh hưởng production (dead code), nhưng confusing
- Fix: Rename key

---

## 4. Proposed Architecture

### 4.1 Target Pipeline (To-Be)
```
Dev → git tag vX.Y.Z (GitLab)
  → GitLab CI: build → test → push Docker image
  → GitLab CI: auto-create MR to bizflycloud-kubernetes-add-ons
      (bump values.yml image tag, INFORMATION.yaml, Chart.yaml)
  → MR review → merge
  → GitLab CI validates addon repo (helm lint, helm template)
  → Staging: install on internal cluster → smoke test
  → Pilot: 1-2 customer clusters → observe 24h
  → matrix_addons.json update → full rollout
```

### 4.2 Helm Values Design (addon repo)

```yaml
# bizflycloud-kubernetes-add-ons/karpenter/values.yml
image:
  repository: cr-hn-1.bizflycloud.vn/31ff9581861a4d0ea4df5e7dda0f665d/karpenter-provider-bizflycloud
  tag: v1.2.3  # bumped by CI

nodeClass:
  nodeCategories: []  # empty = all categories (premium, basic, enterprise, dedicated)
  diskTypes: ["SSD"]
  rootDiskSize: 40
  zones: []  # set via --set from orchestrator

nodePool:
  categories: ["premium", "basic"]
  consolidateAfter: 30s
  expireAfter: 720h
```

### 4.3 Integration Test Strategy

```
test/integration/nodeclaim_test.go:
  1. Apply BizflyCloudNodeClass + NodePool (from testdata/)
  2. Create Pending pod with specific resource request
  3. Wait for NodeClaim to appear (timeout 2m)
  4. Verify NodeClaim.Status.ProviderID != ""
  5. Verify no duplicate NodeClaims (idempotency)
  6. Delete pod → verify NodeClaim deleted (consolidation)
  7. Verify no orphan volumes in Bizfly Cloud API

Env vars required:
  BKE_CLUSTER_ID, BKE_CLUSTER_TOKEN, BIZFLY_APP_CREDENTIAL_ID,
  BIZFLY_APP_CREDENTIAL_SECRET, BKE_NETWORK_ID
```

### 4.4 Rollout Strategy (Blast Radius Controlled)

```
Phase 0: Local build verify (already done — go build exit 0)
Phase 1: Internal cluster (BKE dev/staging)
  - Install via add-on UI
  - Smoke test: create NodePool → trigger scale-out → verify node joins
  - Observe 24h
Phase 2: Pilot (2-3 customer clusters, manual selection)
  - Choose clusters with low workload, easy rollback
  - Monitor: node creation rate, double-node events, disk leak events
  - Rollback: kubectl delete nodepools --all → existing nodes untouched
Phase 3: matrix_addons.json update → available for all clusters
  - Gate: Phase 2 passes 48h without double-node bug
```

---

## 5. File Changes Summary

### `bizflycloud-kubernetes-add-ons` repo

| File | Change |
|------|--------|
| `karpenter/helm/templates/default-nodeclass.yaml` | Add `nodeCategories` field |
| `karpenter/helm/templates/secret.yaml` | Fix `logEndpoint` → `log-endpoint` |
| `karpenter/values.yml` | Add `nodeClass.nodeCategories: []` default |
| `.gitlab-ci.yml` | Add `validate:helm` stage with `helm lint` + `helm template` |

### `karpenter-provider-bizflycloud-2` repo

| File | Change |
|------|--------|
| `.gitlab-ci.yml` (NEW) | Stages: test (go build), build (docker), update-addon (MR) |
| `test/integration/nodeclaim_test.go` (NEW) | Basic smoke test |
| `test/testdata/nodeclass.yaml` (NEW) | Test fixtures |
| `test/testdata/nodepool.yaml` (NEW) | Test fixtures |
| `bke-addon/` (ARCHIVE) | Document as stale reference copy |

### `bke-orchestrator` repo

| File | Change |
|------|--------|
| `app/data/matrix_addons.json` | Add karpenter version mapping per K8s version |

---

## 6. Verification Plan

```bash
# Helm validation
helm lint bizflycloud-kubernetes-add-ons/karpenter/helm/
helm template test bizflycloud-kubernetes-add-ons/karpenter/helm/ \
  --set bizflycloud.apiUrl=https://manage.bizflycloud.vn \
  --set bizflycloud.region=HN \
  --set cluster.id=test-cluster-id \
  --set cluster.kubernetesVersion=1.29 \
  --set cluster.networkId=test-network-id

# Go build
cd karpenter-provider-bizflycloud-2
go build ./...
go vet ./...

# Integration test (requires cluster access)
BKE_CLUSTER_ID=... go test -v -timeout 10m ./test/integration/...
```

---

## 7. Open Questions (Unresolved from Investigation)

1. **`BKE_NETWORK_ID` env in Go binary**: Investigation cannot verify if binary reads this env.
   Orchestrator passes `cluster.networkId` via helm `--set` → `vpcNetworkIds` in NodeClass.
   If NodeClass already has it, Go binary env is redundant. **Assumption: not needed.**

2. **`bke-addons` image source**: `bke-addons:v1.0.6` source repo unknown.
   When Karpenter `deploy.sh` changes, does bke-addons image need rebuild?
   **Assumption: deploy.sh is baked into bke-addons image — version bump triggers image rebuild.**

3. **HCM zones**: If `orchestrator` passes `--set cluster.networkId=<HCM-network>` correctly,
   zones override works. Needs manual verification on HCM cluster.
