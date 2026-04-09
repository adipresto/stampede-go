package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/cucumber/godog"
	"github.com/kaine/stampede/pkg/stampede"
	"github.com/redis/go-redis/v9"
)

type user struct {
	ID   int    `json:"ID"`
	Name string `json:"Name"`
}

type testContext struct {
	mr      *miniredis.Miniredis
	rdb     *redis.Client
	reg     *stampede.Registry
	cache   *stampede.Entity[int, user]
	db      map[int]user
	dbCount int
	lastRes interface{}
	lastErr error

	// For concurrent testing
	concurrentResults []interface{}
	concurrentErrors  []error
	mu                sync.Mutex

	// For HTTP testing
	ts             *httptest.Server
	httpStatus     []int
	httpResponses  []string
}

func (c *testContext) aRedisCacheIsRunning() error {
	mr, err := miniredis.Run()
	if err != nil {
		return err
	}
	c.mr = mr
	c.rdb = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	c.reg = stampede.NewRegistry(c.rdb)
	return nil
}

func (c *testContext) anEntityIsRegisteredWithADBFetcher(prefix string) error {
	c.db = make(map[int]user)
	c.cache = stampede.Register[int, user](c.reg, prefix, func(ctx context.Context, ids []int) (map[int]user, error) {
		c.dbCount++
		res := make(map[int]user)
		for _, id := range ids {
			if u, ok := c.db[id]; ok {
				res[id] = u
			}
		}
		return res, nil
	})
	return nil
}

func (c *testContext) slowEntityRegistration(prefix string) error {
	c.db = make(map[int]user)
	c.cache = stampede.Register[int, user](c.reg, prefix, func(ctx context.Context, ids []int) (map[int]user, error) {
		time.Sleep(500 * time.Millisecond) // Slow DB
		c.dbCount++
		res := make(map[int]user)
		for _, id := range ids {
			if u, ok := c.db[id]; ok {
				res[id] = u
			}
		}
		return res, nil
	})
	return nil
}

func (c *testContext) aNetHTTPServerIsRunning(endpoint string) error {
	// Simple handler using the Stampede cache
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from URL (e.g., /users/55)
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		id, err := strconv.Atoi(parts[2])
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		user, err := c.cache.Get(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	})

	c.ts = httptest.NewServer(handler)
	return nil
}

func (c *testContext) theCacheDoesNotContain(key string) error {
	c.mr.Del(key)
	return nil
}

func (c *testContext) theDatabaseContains(key string) error {
	var id int
	if _, err := fmt.Sscanf(key, "users:id:%d", &id); err == nil {
		c.db[id] = user{ID: id, Name: "StampedeUser"}
	}
	return nil
}

func (c *testContext) theDatabaseContainsWithData(key string, data string) error {
	var id int
	fmt.Sscanf(key, "users:id:%d", &id)
	var u user
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		return err
	}
	c.db[id] = u
	return nil
}

func (c *testContext) theCacheShouldNowContain(key string) error {
	if !c.mr.Exists(key) {
		return fmt.Errorf("expected cache to contain %s", key)
	}
	return nil
}

func (c *testContext) theCacheContainsWithData(key string, data string) error {
	c.mr.Set(key, data)
	return nil
}

func (c *testContext) iRequestEntityWithID(id int) error {
	c.lastRes, c.lastErr = c.cache.Get(context.Background(), id)
	return nil
}

func (c *testContext) iRequestMultipleWithIDs(ids string) error {
	var idList []int
	json.Unmarshal([]byte("["+ids+"]"), &idList)
	c.lastRes, c.lastErr = c.cache.MGet(context.Background(), idList)
	return nil
}

func (c *testContext) concurrentHTTPRequests(count int, path string) error {
	var wg sync.WaitGroup
	wg.Add(count)
	c.mu.Lock()
	c.httpStatus = make([]int, 0, count)
	c.httpResponses = make([]string, 0, count)
	c.mu.Unlock()

	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			resp, err := http.Get(c.ts.URL + path)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			c.mu.Lock()
			c.httpStatus = append(c.httpStatus, resp.StatusCode)
			c.httpResponses = append(c.httpResponses, string(body))
			c.mu.Unlock()
		}()
	}
	wg.Wait()
	return nil
}

func (c *testContext) concurrentRequests(count int, id int) error {
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			res, err := c.cache.Get(context.Background(), id)
			c.mu.Lock()
			c.concurrentResults = append(c.concurrentResults, res)
			c.concurrentErrors = append(c.concurrentErrors, err)
			c.mu.Unlock()
		}()
	}
	wg.Wait()
	return nil
}

func compareJSON(actual interface{}, expectedStr string) error {
	var expectedMap interface{}
	if err := json.Unmarshal([]byte(expectedStr), &expectedMap); err != nil {
		return err
	}

	var actualBytes []byte
	switch v := actual.(type) {
	case string:
		actualBytes = []byte(v)
	default:
		actualBytes, _ = json.Marshal(v)
	}

	var actualMap interface{}
	json.Unmarshal(actualBytes, &actualMap)

	if !reflect.DeepEqual(actualMap, expectedMap) {
		return fmt.Errorf("expected %v, got %v", expectedMap, actualMap)
	}
	return nil
}

func (c *testContext) theResultShouldBe(expectedData string) error {
	if c.lastErr != nil {
		return c.lastErr
	}
	return compareJSON(c.lastRes, expectedData)
}

func (c *testContext) allHTTPStatusShouldBe(expected int) error {
	for i, status := range c.httpStatus {
		if status != expected {
			return fmt.Errorf("request %d: expected status %d, got %d", i, expected, status)
		}
	}
	return nil
}

func (c *testContext) allHTTPResponsesShouldContain(expectedData string) error {
	for i, resp := range c.httpResponses {
		if err := compareJSON(resp, expectedData); err != nil {
			return fmt.Errorf("request %d: %v", i, err)
		}
	}
	return nil
}

func (c *testContext) dbCalledExactlyOnce() error {
	if c.dbCount != 1 {
		return fmt.Errorf("expected 1 DB call, got %d", c.dbCount)
	}
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	c := &testContext{}

	// Lifecycle
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if c.ts != nil {
			c.ts.Close()
		}
		return ctx, nil
	})

	// Steps
	ctx.Step(`^a redis cache is running$`, c.aRedisCacheIsRunning)
	ctx.Step(`^an entity "([^"]*)" is registered with a DB fetcher$`, c.anEntityIsRegisteredWithADBFetcher)
	ctx.Step(`^an entity "([^"]*)" is registered with a DB fetcher that takes 500ms \(slow DB\)$`, c.slowEntityRegistration)
	ctx.Step(`^a net/http server is running with a Stampede-enabled endpoint "([^"]*)"$`, c.aNetHTTPServerIsRunning)
	
	ctx.Step(`^the cache does (?i:not) contain "([^"]*)"$`, c.theCacheDoesNotContain)
	ctx.Step(`^the database contains "([^"]*)"$`, c.theDatabaseContains)
	ctx.Step(`^the database contains "([^"]*)" with data '([^']*)'$`, c.theDatabaseContainsWithData)
	
	ctx.Step(`^I request entity "([^"]*)" with ID (\d+)$`, func(p string, id int) error { return c.iRequestEntityWithID(id) })
	ctx.Step(`^I request multiple "([^"]*)" with IDs \[([\d, ]+)\]$`, func(p string, ids string) error { return c.iRequestMultipleWithIDs(ids) })
	
	ctx.Step(`^(\d+) concurrent requests ask for entity "([^"]*)" with ID (\d+)$`, func(n int, p string, id int) error { return c.concurrentRequests(n, id) })
	ctx.Step(`^(\d+) concurrent HTTP GET requests are made to "([^"]*)"$`, c.concurrentHTTPRequests)
	
	ctx.Step(`^the result should be '([^']*)'$`, c.theResultShouldBe)
	ctx.Step(`^the result should contain (\d+) entities$`, func(n int) error {
		res := c.lastRes.([]user)
		if len(res) != n { return fmt.Errorf("expected %d entities, got %d", n, len(res)) }
		return nil
	})
	ctx.Step(`^entity (\d+) should be '([^']*)'$`, func(id int, data string) error {
		res := c.lastRes.([]user)
		for _, u := range res {
			if u.ID == id { return compareJSON(u, data) }
		}
		return fmt.Errorf("entity %d not found", id)
	})

	ctx.Step(`^all (\d+) HTTP responses should have status (\d+)$`, func(n, s int) error { return c.allHTTPStatusShouldBe(s) })
	ctx.Step(`^all (\d+) HTTP responses should contain '([^']*)'$`, func(n int, d string) error { return c.allHTTPResponsesShouldContain(d) })
	ctx.Step(`^all (\d+) requests should receive '([^']*)'$`, func(n int, d string) error {
		for i, res := range c.concurrentResults {
			if err := c.concurrentErrors[i]; err != nil { return err }
			if err := compareJSON(res, d); err != nil { return err }
		}
		return nil
	})

	ctx.Step(`^the database fetcher should only have been called EXACTLY 1 time$`, c.dbCalledExactlyOnce)
	ctx.Step(`^the database should NOT be called$`, func() error {
		if c.dbCount > 0 { return fmt.Errorf("DB was called") }
		return nil
	})
	ctx.Step(`^the database should only be queried for ID \[([\d, ]+)\]$`, func(ids string) error { return c.dbCalledExactlyOnce() })
	ctx.Step(`^the cache should now contain "([^"]*)"$`, c.theCacheShouldNowContain)
	ctx.Step(`^the cache contains "([^"]*)" with data '([^']*)'$`, c.theCacheContainsWithData)
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("failed to run feature tests")
	}
}
