// Copyright 2021 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package db

import (
	"context"
	"github.com/xmidt-org/ears/internal/pkg/config"
	"github.com/xmidt-org/ears/pkg/route"
	"github.com/xmidt-org/ears/pkg/tenant"
	"sync"
	"time"
)

type InMemoryRouteStorer struct {
	tenants map[string]map[string]*route.Config
	lock    *sync.RWMutex
}

func NewInMemoryRouteStorer(config config.Config) *InMemoryRouteStorer {
	return &InMemoryRouteStorer{
		tenants: make(map[string]map[string]*route.Config),
		lock:    &sync.RWMutex{},
	}
}

func (s *InMemoryRouteStorer) GetAllRoutes(ctx context.Context) ([]route.Config, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	routes := make([]route.Config, 0)
	for _, tenant := range s.tenants {
		for _, r := range tenant {
			routes = append(routes, *r)
		}
	}
	return routes, nil
}

func (s *InMemoryRouteStorer) GetRoute(ctx context.Context, tid tenant.Id, id string) (route.Config, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	empty := route.Config{}

	t, ok := s.tenants[tid.String()]
	if !ok {
		return empty, &route.RouteNotFoundError{tid, id}
	}

	r, ok := t[id]
	if !ok {
		return empty, &route.RouteNotFoundError{tid, id}
	}

	newCopy := *r
	return newCopy, nil
}

func (s *InMemoryRouteStorer) GetAllTenantRoutes(ctx context.Context, id tenant.Id) ([]route.Config, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	routes := make([]route.Config, 0)
	t, ok := s.tenants[id.String()]
	if !ok {
		return routes, nil
	}

	for _, r := range t {
		routes = append(routes, *r)
	}
	return routes, nil
}

func (s *InMemoryRouteStorer) setRoute(r route.Config) {
	r.Modified = time.Now().Unix()
	var tenant map[string]*route.Config
	if t, ok := s.tenants[r.TenantId.String()]; !ok {
		tenant = make(map[string]*route.Config)
		s.tenants[r.TenantId.String()] = tenant
	} else {
		tenant = t
	}

	if existing, ok := tenant[r.Id]; !ok {
		r.Created = r.Modified
	} else {
		r.Created = existing.Created
	}
	tenant[r.Id] = &r
}

func (s *InMemoryRouteStorer) SetRoute(ctx context.Context, r route.Config) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.setRoute(r)
	return nil
}

func (s *InMemoryRouteStorer) SetRoutes(ctx context.Context, routes []route.Config) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, r := range routes {
		s.setRoute(r)
	}
	return nil
}

func (s *InMemoryRouteStorer) DeleteRoute(ctx context.Context, tid tenant.Id, id string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	t, ok := s.tenants[tid.String()]
	if !ok {
		return nil
	}

	delete(t, id)
	return nil
}

func (s *InMemoryRouteStorer) DeleteRoutes(ctx context.Context, tid tenant.Id, ids []string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	t, ok := s.tenants[tid.String()]
	if !ok {
		return nil
	}

	for _, id := range ids {
		delete(t, id)
	}
	return nil
}
