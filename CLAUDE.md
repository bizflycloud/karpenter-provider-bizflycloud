# Karpenter Provider for BizFly Cloud

As a Kubernetes expert with extensive experience in cloud provider integrations, I need your assistance to design and implement a custom Karpenter provider for **BizFly Cloud**. This provider will enable Kubernetes auto-scaling capabilities on BizFly Cloud infrastructure.

## Architecture & Structure

- Follow the same architectural patterns used in the Proxmox provider (https://github.com/sergelogvinov/karpenter-provider-proxmox).
- Implement the Karpenter Provider API interfaces required for node provisioning and termination.
- Organize code following Go best practices and mirror the structure of the reference implementation:
  - `internal/` for core logic
  - `cmd/` for CLI entrypoints
  - `pkg/` for shared utilities
  - `config/` for default settings and CRD definitions

## Core Functionality

1. **Node Provisioning**
   - Use the BizFly Cloud Go client (`github.com/bizflycloud/gobizfly`) to interact with the BizFly API.
   - Implement dynamic creation of cloud server instances based on Karpenter’s `ProvisionerSpec`.

2. **Node Termination**
   - Gracefully drain and delete instances when scale-down events occur.
   - Ensure proper teardown of associated resources (network interfaces, storage volumes).

3. **Integration with Kubernetes**
   - Expose metrics and health checks required by the Karpenter core controller.
   - Ensure seamless scaling by handling `NodeTemplate`, `Offerings`, and `Webhook` configurations.

4. **Instance Type Selection**
   - Map Karpenter’s resource requests (CPU, memory, GPU) to BizFly Cloud instance flavors.
   - Implement a selection algorithm that balances cost, resource availability, and provisioning speed.

## Feature Parity & Adaptations

Adapt and enhance the following features from the Proxmox provider to work with BizFly Cloud:

- **Machine Template Management**
  - Translate Proxmox VM templates into BizFly Cloud server images and snapshots.

- **Resource Allocation Strategies**
  - Implement strategies for bin-packing, spread, and custom label constraints using BizFly zones.

- **Drift Detection**
  - Continuously compare actual instance specs with desired state; auto-reconcile drift by patching or recreating instances.

- **Capacity-Aware Scheduling**
  - Query real-time capacity from BizFly API to avoid provisioning failures due to insufficient resources.

## Integration & Authentication

- **Authentication**
  - Support API key and secret authentication via environment variables or Kubernetes `Secret`.
  - Use `gobizfly.NewClient` with secure TLS configuration.

- **Configuration Management**
  - Define a `ProviderConfig` CRD to store endpoint, credentials, and default region.
  - Leverage Kubernetes `ConfigMap` for optional tuning parameters.

- **Clean Interface**
  - Provide a thin adapter layer between Karpenter core types and Gobizfly client calls.
  - Abstract API operations under an interface for easier testing and mocking.

- **Kubernetes Intergration**
  - Provide all the nessecary CRD, deployment, secrets, ...
  - Provide helm chart for install

## Documentation & Testing

- **Documentation**
  - Write a comprehensive `README.md` covering:
    - Prerequisites and setup
    - Installation steps using `kubectl apply`
    - Configuration examples for different workload scenarios
  - Include inline GoDoc comments for public types and methods.

- **Testing**
  - Implement unit tests for:
    - Instance creation and deletion logic
    - Flavor selection algorithm
    - Authentication and error handling
  - Add integration tests (using a BizFly Cloud sandbox) to validate end-to-end provisioning and teardown.

- **Usage Examples**
  - Provide sample CRDs demonstrating:
    - Custom `Provisioner` with zone and instance type overrides
    - Scale-to-zero and burst scaling scenarios

- **Docker image and push**
  - Build the image: cr-hn-1.bizflycloud.vn/31ff9581861a4d0ea4df5e7dda0f665d/karpenter-provider-bizflycloud with custom tag as you choose for developement

---

**Implementation Approach**

I will start by scaffolding a new Go module mirroring the Proxmox provider’s directory layout. Then, I’ll define the Provider API interface implementations and stub out basic create/delete calls using the Gobizfly client. Next, I’ll adapt resource allocation and drift detection logic to BizFly Cloud’s API model. Afterward, I’ll integrate authentication and configuration through Kubernetes CRDs and secrets. Finally, I’ll write thorough documentation, unit tests, and integration tests to ensure production readiness.

1. **Complete BizFly Cloud Integration**:
   - Finalize integration with the actual BizFly Cloud API
   - Implement error handling and retry logic for API calls
   - Add support for more advanced BizFly Cloud features
   - Implement metadata and tags for better resource tracking

2. **Enhanced Testing**:
   - Add more comprehensive unit tests
   - Implement integration tests with BizFly Cloud sandbox
   - Add e2e tests for full lifecycle validation

3. **Documentation**:
   - Add detailed usage examples
   - Document configuration options and best practices
   - Add troubleshooting guides

4. **CI/CD**:
   - Set up CI/CD pipeline for automated testing and releases
   - Implement version management and release process