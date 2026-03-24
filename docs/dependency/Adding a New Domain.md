# Adding a New Domain

This document covers adding an entirely new domain to the application when no related code exists yet.

---

## Files to Create or Modify

| # | File | Action |
|---|------|--------|
| 1 | `internal/models/<domain>.go` | Create model struct |
| 2 | `internal/infrastructure/db/<domain>_repository.go` | Create repository (if needed) |
| 3 | `internal/service/<domain>_service.go` | Create service + define repo interface (if needed) |
| 4 | `internal/service/container.go` | Add field + update `NewContainer` params |
| 5 | `internal/api/handlers/<domain>.go` | Add handler methods on `Handler` |
| 6 | `internal/api/routes.go` | Add URL mapping |
| 7 | `internal/app/app.go` | Wire repo, service, update container call |

---

## 1. Model

```go
// internal/models/<domain>.go
package models

type Name struct {
    // fields here
}
```

---

## 2. Repository

```go
// internal/infrastructure/db/<domain>_repository.go
package db

type NameRepository struct {
    db *sql.DB
}

func NewNameRepository(db *sql.DB) *NameRepository {
    return &NameRepository{db: db}
}

func (r *NameRepository) MethodName() error {
    // code here
}
```

---

## 3. Service

```go
// internal/service/<domain>_service.go
package service

type NameRepository interface {
    MethodName() error
}

type NameService struct {
    repo NameRepository
    logg *logger.Logger
}

func NewNameService(repo NameRepository, logg *logger.Logger) *NameService {
    return &NameService{repo: repo, logg: logg}
}

func (s *NameService) MethodName() error {
    // code here
}
```

---

## 4. Service Container

```go
// internal/service/container.go

type Container struct {
    // ... existing fields
    Name *NameService // add
}

func NewContainer(/* existing params */, name *NameService) *Container {
    return &Container{
        // ... existing assignments
        Name: name, // add
    }
}
```

---

## 5. Handler

```go
// internal/api/handlers/<domain>.go
package handlers

func (h *Handler) MethodName(c echo.Context) error {
    // code here
}
```

---

## 6. Routes

```go
// internal/api/routes.go

api.GET("/<path>", h.MethodName) // add
```

---

## 7. App Wiring

```go
// internal/app/app.go

// Infrastructure
nameRepo := infradb.NewNameRepository(database)

// Services
nameSvc := service.NewNameService(nameRepo, logg)

// Service Container (update existing call)
services := service.NewContainer(/* existing params */, nameSvc)
```

---

## Service Variations

Not every service needs a repository or a model. Below are the valid patterns.

### Service with Repository

The standard pattern. Service holds business logic, repository handles persistence.

```go
type NameService struct {
    repo NameRepository
    logg *logger.Logger
}

func NewNameService(repo NameRepository, logg *logger.Logger) *NameService {
    return &NameService{repo: repo, logg: logg}
}
```

**App wiring:**

```go
nameRepo := infradb.NewNameRepository(database)
nameSvc := service.NewNameService(nameRepo, logg)
```

### Service with No Repository

For services that perform logic without any persistence — e.g., encryption, token generation, validation.

```go
type NameService struct {
    logg *logger.Logger
}

func NewNameService(logg *logger.Logger) *NameService {
    return &NameService{logg: logg}
}
```

**App wiring:**

```go
nameSvc := service.NewNameService(logg)
```

No infrastructure file created. Skip step 2.

### Service with Multiple Repositories

For services that coordinate across multiple data sources.

```go
type NameService struct {
    repoA NameARepository
    repoB NameBRepository
    logg  *logger.Logger
}

func NewNameService(a NameARepository, b NameBRepository, logg *logger.Logger) *NameService {
    return &NameService{repoA: a, repoB: b, logg: logg}
}
```

**App wiring:**

```go
repoA := infradb.NewNameARepository(database)
repoB := infradb.NewNameBRepository(database)
nameSvc := service.NewNameService(repoA, repoB, logg)
```

### Service with Non-DB Infrastructure

For services that depend on infrastructure other than a database — e.g., network utilities, external APIs.

```go
type NameService struct {
    resolver NetworkResolver
}

func NewNameService(resolver NetworkResolver) *NameService {
    return &NameService{resolver: resolver}
}
```

The interface is still defined in the service file. The implementation lives in `internal/infrastructure/network/` or whichever infrastructure subdirectory is appropriate.

### Service with No Model

Some domains don't need a dedicated model — e.g., health checks, connectivity probes. The service returns primitive types or inline maps. Skip step 1.

### Handler with No Service

In rare cases a handler may not need a service at all — e.g., a static page redirect, a version endpoint returning a hardcoded value. The handler method still lives on `Handler` but doesn't call into `h.Services`. Skip steps 2, 3, 4, and 7.

---

## Summary

| Variation | Model | Repo | Service | Container | App Wiring |
|-----------|-------|------|---------|-----------|------------|
| Standard | ✅ | ✅ | ✅ | ✅ | ✅ |
| No repo | Optional | ❌ | ✅ | ✅ | ✅ (no repo line) |
| Multiple repos | Optional | ✅ (multiple) | ✅ | ✅ | ✅ |
| Non-DB infra | Optional | ❌ (infra instead) | ✅ | ✅ | ✅ |
| No model | ❌ | Optional | ✅ | ✅ | ✅ |
| No service | Optional | ❌ | ❌ | ❌ | ❌ |