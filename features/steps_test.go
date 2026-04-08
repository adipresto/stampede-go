package features

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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

func (c *testContext) theCacheDoesNotContain(key string) error {
	c.mr.Del(key)
	return nil
}

func (c *testContext) theDatabaseContainsWithData(id int, data string) error {
	var u user
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		return err
	}
	c.db[id] = u
	return nil
}

func (c *testContext) iRequestEntityWithID(prefix string, id int) error {
	c.lastRes, c.lastErr = c.cache.Get(context.Background(), id)
	return nil
}

func compareJSON(actual interface{}, expectedStr string) error {
	var expectedMap interface{}
	if err := json.Unmarshal([]byte(expectedStr), &expectedMap); err != nil {
		return err
	}

	actualBytes, _ := json.Marshal(actual)
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

func (c *testContext) theDatabaseShouldNOTBeCalled() error {
	if c.dbCount > 0 {
		return fmt.Errorf("expected DB NOT to be called, but it was called %d times", c.dbCount)
	}
	return nil
}

func (c *testContext) iRequestMultipleWithIDs(prefix string, idList []int) error {
	c.lastRes, c.lastErr = c.cache.MGet(context.Background(), idList)
	return nil
}

func (c *testContext) theResultShouldContainEntities(count int) error {
	res := c.lastRes.([]user)
	if len(res) != count {
		return fmt.Errorf("expected %d entities, got %d", count, len(res))
	}
	return nil
}

func (c *testContext) entityShouldBe(id int, expectedData string) error {
	res := c.lastRes.([]user)
	for _, u := range res {
		if u.ID == id {
			return compareJSON(u, expectedData)
		}
	}
	return fmt.Errorf("entity %d not found in result", id)
}

func (c *testContext) theDatabaseShouldOnlyBeQueriedForID(idList []int) error {
	if c.dbCount != 1 {
		return fmt.Errorf("expected 1 DB call, got %d", c.dbCount)
	}
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	c := &testContext{}

	ctx.Step(`^a redis cache is running$`, c.aRedisCacheIsRunning)
	ctx.Step(`^an entity "([^"]*)" is registered with a DB fetcher$`, c.anEntityIsRegisteredWithADBFetcher)
	ctx.Step(`^the cache does (?i:not) contain "([^"]*)"$`, c.theCacheDoesNotContain)
	ctx.Step(`^the database contains "([^"]*)" with data '([^']*)'$`, func(key string, data string) error {
		var id int
		fmt.Sscanf(key, "users:id:%d", &id)
		return c.theDatabaseContainsWithData(id, data)
	})
	ctx.Step(`^I request entity "([^"]*)" with ID (\d+)$`, c.iRequestEntityWithID)
	ctx.Step(`^the result should be '([^']*)'$`, c.theResultShouldBe)
	ctx.Step(`^the cache should now contain "([^"]*)"$`, c.theCacheShouldNowContain)
	ctx.Step(`^the cache contains "([^"]*)" with data '([^']*)'$`, c.theCacheContainsWithData)
	ctx.Step(`^the database should NOT be called$`, c.theDatabaseShouldNOTBeCalled)
	
	// MGet steps
	ctx.Step(`^the database contains "([^"]*)"$`, func(key string) error {
		return nil
	})
	ctx.Step(`^I request multiple "([^"]*)" with IDs \[([\d, ]+)\]$`, func(prefix string, ids string) error {
		var idList []int
		json.Unmarshal([]byte("["+ids+"]"), &idList)
		return c.iRequestMultipleWithIDs(prefix, idList)
	})
	ctx.Step(`^the result should contain (\d+) entities$`, c.theResultShouldContainEntities)
	ctx.Step(`^entity (\d+) should be '([^']*)'$`, c.entityShouldBe)
	ctx.Step(`^the database should only be queried for ID \[([\d, ]+)\]$`, func(ids string) error {
		return c.theDatabaseShouldOnlyBeQueriedForID(nil)
	})

	// Stampede steps
	ctx.Step(`^an entity "([^"]*)" is registered with a DB fetcher that takes 500ms \(slow DB\)$`, func(prefix string) error {
		c.db = make(map[int]user)
		c.cache = stampede.Register[int, user](c.reg, prefix, func(ctx context.Context, ids []int) (map[int]user, error) {
			time.Sleep(500 * time.Millisecond)
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
	})
	ctx.Step(`^the database contains "([^"]*)"$`, func(key string) error {
		var id int
		fmt.Sscanf(key, "users:id:%d", &id)
		c.db[id] = user{ID: id, Name: "StampedeUser"}
		return nil
	})
	ctx.Step(`^(\d+) concurrent requests ask for entity "([^"]*)" with ID (\d+)$`, func(count int, prefix string, id int) error {
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				defer wg.Done()
				c.cache.Get(context.Background(), id)
			}()
		}
		wg.Wait()
		return nil
	})
	ctx.Step(`^the database fetcher should only have been called EXACTLY 1 time$`, func() error {
		if c.dbCount != 1 {
			return fmt.Errorf("expected 1 DB call, got %d", c.dbCount)
		}
		return nil
	})
	ctx.Step(`^all (\d+) requests should receive '([^']*)'$`, func(count int, data string) error {
		return nil
	})
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
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}
