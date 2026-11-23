package sqlbase

import (
	"context"
	"errors"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// Optional dialect hooks for server administration.

type RoleDialect interface {
	Roles(ctx context.Context, e *Engine) ([]driver.Role, error)
	SaveRole(ctx context.Context, e *Engine, ch driver.RoleChange) error
	DropRole(ctx context.Context, e *Engine, name string) error
}

type DatabaseDialect interface {
	CreateDatabase(ctx context.Context, e *Engine, name, owner string) error
	DropDatabase(ctx context.Context, e *Engine, name string) error
}

type StructureDialect interface {
	Structure(ctx context.Context, e *Engine, path []string) (*driver.TableStructure, error)
}

var errUnsupported = errors.New("not supported for this database")

func (e *Engine) Roles(ctx context.Context) ([]driver.Role, error) {
	if d, ok := e.Dialect.(RoleDialect); ok {
		return d.Roles(ctx, e)
	}
	return nil, errUnsupported
}

func (e *Engine) SaveRole(ctx context.Context, ch driver.RoleChange) error {
	if d, ok := e.Dialect.(RoleDialect); ok {
		return d.SaveRole(ctx, e, ch)
	}
	return errUnsupported
}

func (e *Engine) DropRole(ctx context.Context, name string) error {
	if d, ok := e.Dialect.(RoleDialect); ok {
		return d.DropRole(ctx, e, name)
	}
	return errUnsupported
}

func (e *Engine) CreateDatabase(ctx context.Context, name, owner string) error {
	if d, ok := e.Dialect.(DatabaseDialect); ok {
		return d.CreateDatabase(ctx, e, name, owner)
	}
	return errUnsupported
}

func (e *Engine) DropDatabase(ctx context.Context, name string) error {
	d, ok := e.Dialect.(DatabaseDialect)
	if !ok {
		return errUnsupported
	}
	e.ClosePool(name) // our own sessions would block the drop
	return d.DropDatabase(ctx, e, name)
}

func (e *Engine) Structure(ctx context.Context, path []string) (*driver.TableStructure, error) {
	if d, ok := e.Dialect.(StructureDialect); ok {
		return d.Structure(ctx, e, path)
	}
	return nil, errUnsupported
}

// ClosePool closes the cached per-database pool for name, if any.
func (e *Engine) ClosePool(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.pools[name]; ok {
		_ = p.Close()
		delete(e.pools, name)
	}
}
