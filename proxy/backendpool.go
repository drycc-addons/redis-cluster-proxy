package proxy

import (
	"sync"
	"time"

	"github.com/drycc-addons/redis-cluster-proxy/proxy/connpool"
)

type BackendServerPool struct {
	lock           sync.Mutex
	redisConn      *RedisConn
	backendServers sync.Map
}

func NewBackendServerPool(redisConn *RedisConn) *BackendServerPool {
	return &BackendServerPool{redisConn: redisConn}
}

func (b *BackendServerPool) Init(server string) (*connpool.Pool, error) {
	pool, err := connpool.NewChannelPool(&connpool.Config{
		InitCap: b.redisConn.initCap,
		MaxIdle: b.redisConn.maxIdle,
		Factory: func() (interface{}, error) {
			return NewBackendServer(server, b.redisConn), nil
		},
		Close:       func(v interface{}) error { return v.(*BackendServer).Close() },
		IdleTimeout: 60 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	b.backendServers.Store(server, &pool)
	return &pool, nil
}

func (b *BackendServerPool) Get(server string) (*BackendServer, error) {
	var err error
	var pool *connpool.Pool
	value, ok := b.backendServers.Load(server)
	if ok {
		pool = value.(*connpool.Pool)
	} else {
		b.lock.Lock()
		defer b.lock.Unlock()
		value, ok := b.backendServers.Load(server)
		if !ok {
			pool, err = b.Init(server)
			if err != nil {
				return nil, err
			}
		} else {
			pool = value.(*connpool.Pool)
		}
	}
	backendServer, err := (*pool).Get()
	return backendServer.(*BackendServer), err
}

func (b *BackendServerPool) Put(server *BackendServer) error {
	value, ok := b.backendServers.Load(server.server)
	if ok {
		pool := *(value.(*connpool.Pool))
		return pool.Put(server)
	}
	return nil
}

func (b *BackendServerPool) Reload(servers map[string]bool) {
	b.backendServers.Range(func(key, value any) bool {
		server, pool := key.(string), *(value.(*connpool.Pool))
		if _, ok := servers[server]; !ok {
			pool.Release()
			b.backendServers.Delete(server)
		}
		return true
	})
}
