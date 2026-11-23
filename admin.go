package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// Server administration (roles, databases, table structure).

// capable returns the open connection if its driver advertises capability;
// write=true also rejects read-only connections.
func (a *App) capable(id, capability string, write bool) (driver.Conn, context.Context, context.CancelFunc, error) {
	p, err := a.store.Get(id)
	if err != nil {
		return nil, nil, nil, err
	}
	d, err := driver.Get(p.Driver)
	if err != nil {
		return nil, nil, nil, err
	}
	if !slices.Contains(d.Info().Capabilities, capability) {
		return nil, nil, nil, fmt.Errorf("%s doesn't support this yet", d.Info().Name)
	}
	if write && p.ReadOnly {
		return nil, nil, nil, errors.New("this connection is read-only — edit it to allow changes")
	}
	c, err := a.sessions.Get(id)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	return c, ctx, cancel, nil
}

func (a *App) ListRoles(id string) ([]driver.Role, error) {
	c, ctx, cancel, err := a.capable(id, driver.CapRoles, false)
	if err != nil {
		return nil, err
	}
	defer cancel()
	roles, err := c.(driver.RoleManager).Roles(ctx)
	if roles == nil {
		roles = []driver.Role{}
	}
	return roles, err
}

func (a *App) SaveRole(id string, ch driver.RoleChange) error {
	c, ctx, cancel, err := a.capable(id, driver.CapRoles, true)
	if err != nil {
		return err
	}
	defer cancel()
	return c.(driver.RoleManager).SaveRole(ctx, ch)
}

func (a *App) DropRole(id, name string) error {
	c, ctx, cancel, err := a.capable(id, driver.CapRoles, true)
	if err != nil {
		return err
	}
	defer cancel()
	return c.(driver.RoleManager).DropRole(ctx, name)
}

func (a *App) CreateDatabase(id, name, owner string) error {
	c, ctx, cancel, err := a.capable(id, driver.CapDatabases, true)
	if err != nil {
		return err
	}
	defer cancel()
	return c.(driver.DatabaseManager).CreateDatabase(ctx, name, owner)
}

func (a *App) DropDatabase(id, name string) error {
	c, ctx, cancel, err := a.capable(id, driver.CapDatabases, true)
	if err != nil {
		return err
	}
	defer cancel()
	return c.(driver.DatabaseManager).DropDatabase(ctx, name)
}

func (a *App) Structure(id string, path []string) (*driver.TableStructure, error) {
	c, ctx, cancel, err := a.capable(id, driver.CapStructure, false)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return c.(driver.Structurer).Structure(ctx, path)
}
