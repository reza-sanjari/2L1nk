# Adding to an Existing Domain

This document covers adding a new method to a domain that is already wired into the application.

The service container and app wiring are not touched.

---

## Files Touched

| # | File | Action |
|---|------|--------|
| 1 | `internal/infrastructure/db/<domain>_repository.go` | Add repository method (if needed) |
| 2 | `internal/service/<domain>_service.go` | Add service method + update repo interface (if needed) |
| 3 | `internal/api/handlers/<domain>.go` | Add handler method |
| 4 | `internal/api/routes.go` | Add URL mapping |

## Files Not Touched

| File | Reason |
|------|--------|
| `internal/service/container.go` | Service already registered |
| `internal/app/app.go` | Service already wired |
| `internal/models/<domain>.go` | Unchanged unless new fields are needed |

---

## 1. Repository Method

```go
// internal/infrastructure/db/<domain>_repository.go

func (r *NameRepository) NewMethodName() error {
    // code here
}
```

If the service's repository interface does not yet include this method, update it in the service file:

```go
// internal/service/<domain>_service.go

type NameRepository interface {
    // ... existing methods
    NewMethodName() error // add
}
```

---

## 2. Service Method

```go
// internal/service/<domain>_service.go

func (s *NameService) NewMethodName() error {
    // code here
}
```

---

## 3. Handler Method

```go
// internal/api/handlers/<domain>.go

func (h *Handler) NewMethodName(c echo.Context) error {
    // code here
}
```

---

## 4. Route

```go
// internal/api/routes.go

api.GET("/<path>", h.NewMethodName) // add
```

---

## Variations

### Adding Only a Handler Method

The new endpoint calls an existing service method or returns a response without calling any service. Only steps 3 and 4 are needed.

### Adding Only a Service Method

New business logic that uses existing repository methods. Only step 2 is needed. No handler or route until an endpoint is required.

### Adding Only a Repository Method

New query or persistence logic needed by an existing service method, or in preparation for a future service method. Only step 1 is needed. Update the repo interface in the service file if the service will call it.

### Adding a New Model Field

If the new method requires additional data on the model, update the model struct in `internal/models/<domain>.go`. This may also require updating the repository methods that read or write that field.