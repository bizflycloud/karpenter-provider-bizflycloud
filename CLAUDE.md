# Karpenter BizflyCloud Provider - Development Guide

## Project Overview
This is a Kubernetes Karpenter provider for BizflyCloud, implementing node provisioning and lifecycle management. The project follows Kubernetes controller patterns and uses the controller-runtime framework.

## Core Commands

### Build & Development
- `make build`: Build the controller binary
- `make docker-build`: Build Docker image locally
- `make docker-push`: Push Docker image to registry
- `docker build --build-arg VERSION=v1.0.0 --build-arg BUILDDATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') --platform linux/amd64 -t cr-hn-1.bizflycloud.vn/cecd854937c8421b81d1d73789acad52/karpenter:v1.2.0 . --push`: Production build command

### Testing & Quality
- `make test`: Run unit tests
- `make lint`: Run linting checks
- `make vet`: Run go vet
- `go test ./... -v`: Verbose test execution

## Code Style Guidelines

### Imports Organization
- Standard library imports first
- External dependencies second  
- Internal packages last
- Use blank lines to separate groups

### Error Handling
- Return errors explicitly with context
- Use `fmt.Errorf()` for wrapping errors
- Prefer `errors.Is()` and `errors.As()` for error checking

### Naming Conventions
- CamelCase for exported functions/types
- camelCase for unexported functions/types
- Use descriptive names that explain purpose
- Interface names should end with 'er' when appropriate

### Testing Standards
- Use YAML test files in `testdata` directory
- Leverage foxytest package for test utilities
- Write table-driven tests for multiple scenarios
- Mock external dependencies using interfaces

### Type System
- Use strong typing throughout
- Prefer interfaces for dependencies to enable testing
- Define clear contracts between components

### Documentation Requirements
- Document all exported functions and types with godoc
- Include usage examples for complex functions
- Maintain up-to-date README.md

### Project Structure
- Follow Kubernetes-like API structure in `internal/k8s` package
- Use dependency injection with fx framework
- Pass Kubernetes contexts explicitly as parameters

## Refactoring Instructions

When analyzing and refactoring this codebase:

1. **Break Down Monoliths**: Split large files into focused, single-responsibility modules
2. **Improve Separation of Concerns**: Ensure each package has a clear, distinct purpose
3. **Extract Common Utilities**: Move shared functionality to dedicated utility packages
4. **Enhance Testability**: Create interfaces for external dependencies
5. **Optimize Package Structure**: Follow Go best practices for package organization

## Suggested Improved Directory Structure

```
├── cmd/
│ └── karpenter-bizflycloud/ # Main application entry point
│ └── main.go
├── config/
│ ├── crd/ # Custom Resource Definitions
│ ├── rbac/ # RBAC configurations
│ └── samples/ # Example configurations
├── internal/
│ ├── auth/ # Authentication logic
│ ├── config/ # Configuration management
│ ├── metrics/ # Metrics and monitoring
│ ├── logging/ # Structured logging utilities
│ ├── errors/ # Custom error types
│ └── testing/ # Test utilities and mocks
├── pkg/
│ ├── apis/
│ │ └── v1/ # API types and validation
│ ├── client/ # Kubernetes client wrappers
│ ├── cloudprovider/
│ │ ├── bizflycloud/ # BizflyCloud-specific implementation
│ │ └── interface.go # Cloud provider interface
│ ├── controllers/
│ │ ├── nodeclass/ # NodeClass controller
│ │ └── common/ # Shared controller utilities
│ ├── operator/ # Operator lifecycle management
│ ├── providers/
│ │ ├── capacity/ # Cloud capacity management
│ │ ├── instance/ # Instance management
│ │ └── pricing/ # Pricing calculations
│ └── utils/ # Common utilities
├── test/
│ ├── integration/ # Integration tests
│ ├── e2e/ # End-to-end tests
│ └── testdata/ # Test data files
├── docs/ # Documentation
├── scripts/ # Build and deployment scripts
└── hack/ # Development tools and scripts
```


## Logging Improvements

Implement structured logging with the following standards:
- Use structured logging (JSON format for production)
- Include correlation IDs for request tracing
- Log at appropriate levels (Debug, Info, Warn, Error)
- Include relevant context (node names, instance IDs, etc.)
- Avoid logging sensitive information (credentials, tokens)

## Documentation Requirements

Generate the following documentation:
1. **README.md**: Project overview, installation, and usage
2. **CONTRIBUTING.md**: Development guidelines and contribution process
3. **API.md**: API reference and examples
4. **DEPLOYMENT.md**: Deployment instructions and configurations
5. **TROUBLESHOOTING.md**: Common issues and solutions

## Automated Tasks

When working on this project, please:

1. **Analyze Current Structure**: 
   - Read all files and understand the current architecture
   - Identify monolithic components that need breaking down
   - Map dependencies between packages

2. **Refactor Code**:
   - Split large files into focused modules
   - Extract common functionality into utilities
   - Improve error handling and logging
   - Add missing interfaces for testability

3. **Improve Project Structure**:
   - Reorganize packages according to the suggested structure
   - Move files to appropriate directories
   - Update import paths accordingly

4. **Generate Documentation**:
   - Create comprehensive README.md
   - Add godoc comments to all exported functions
   - Generate API documentation
   - Create deployment and troubleshooting guides

5. **Enhance Testing**:
   - Add unit tests for all packages
   - Create integration tests for controllers
   - Set up test utilities and mocks

## Quality Checks

Before completing any refactoring:
- Ensure all tests pass
- Run linting and formatting tools
- Verify Docker build succeeds
- Check that all imports are properly organized
- Validate that error handling follows project standards

## Dependencies

Key external dependencies:
- controller-runtime: Kubernetes controller framework
- client-go: Kubernetes API client
- fx: Dependency injection framework
- foxytest: Testing utilities
- BizflyCloud SDK: Cloud provider API client

