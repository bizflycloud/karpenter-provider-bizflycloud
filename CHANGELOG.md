# Changelog

## 2026-10-18 — Karpenter BizflyCloud Fixes

### Critical Bugs Fixed

| Bug | File | Fix |
|-----|------|-----|
| Tạo 2 node trùng khi đang đợi boot | `cloudprovider.go` | Xóa `throttleNodeCreation()` mutex 300s |
| Tạo 2 node trùng khi restart | `manager.go` | Idempotency check: list servers filter `name+clusterID+Status!=ERROR/DELETED` |
| Root disk volume leak sau scale-down | `manager.go:DeleteInstance` | Collect `AttachedVolumes[rootdisk]` → xóa cùng VM |
| NodeClaim stuck 5 phút khi delete | `cloudprovider.go:Delete` | 404 → `NewNodeClaimNotFoundError()` ngay lập tức |

### Features / Improvements

| Feature | File | Detail |
|---------|------|--------|
| `EnablePublicIP *bool` | `bizflycloudnodeclass.go` | Optional field, nil=true (WAN IP mặc định bật) |
| `Template` optional | `bizflycloudnodeclass.go` | Không còn required |
| capacity-type normalize | `manager.go` | `on-demand`→`on_demand`, `saving-plan`→`saving_plan`, `spot`→`spot_instance` |
| diskType fallback chain | `manager.go` | requirement → `NodeClass.DiskTypes[0]` → `"SSD"` |
| vpcNetworkIDs fallback | `manager.go` | `NodeClass.VPCNetworkIDs` → `BKE_NETWORK_ID` env → error |
| SecurityGroups | `manager.go` | Đọc từ `NodeClass.Spec.SecurityGroups` → Firewalls |
| Sort deterministic | `cloudprovider.go` | `Name` tie-breaker trong `getSmallestInstanceTypeFromRequirements` |
| ClusterID từ env | `manager.go:NewManager` | `os.Getenv("BKE_CLUSTER_ID")` |
| List() từ Cloud API | `cloudprovider.go` | Filter `karpenter-managed=true && bke_cluster_id` — không đọc K8s NodeList |
| Pod capacity = 110 | `capacity.go` | Cố định khớp BKE kubelet config |
| Overhead formula đúng | `capacity.go` | SystemReserved+KubeReserved lũy tiến, EvictionThreshold=100Mi |
| ParseCPUVendor dynamic | `parser.go` | HasSuffix("a")=AMD else Intel — hỗ trợ mọi thế hệ |

### bke-orchestrator (Python) — 2026-10-18

| Bug | File | Fix |
|-----|------|-----|
| `network_id` sai field | `cluster_updater.py:536` | → `private_network_id` |
| `this.shoot.token` missing attr | `cluster_updater.py:574` | → `getattr(this.shoot, 'cluster_token', '') or ''` |
| `CLOUD_API_URL` sai config key | `cluster_updater.py:575-576` | → `BKE_API_URL` cho join/log endpoint |
| Thiếu `local-dns` trong Secret | `cluster_updater.py:579` | Thêm key `local-dns` từ `shoot.local_dns` |
| Filter VM Karpenter theo `tags` (sai) | `cluster_destroyer.py:214-226` | → `server.metadata` (karpenter-managed, bke_cluster_id) |
| Root disk không được xóa khi destroy | `cluster_destroyer.py` | Xóa `attached_volumes[rootdisk]` trước khi xóa VM |

### Verification

```
go build ./...          → exit 0 ✅
go test -v ./pkg/...    → [no test files] ✅
```
