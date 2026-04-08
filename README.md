# Stampede

**Stampede** is a high-performance, developer-friendly Redis wrapper for Go, specifically designed to solve the **Thundering Herd (Cache Stampede)** problem and optimize **ID Projection** patterns in massive datasets.

## Key Features

-   🚀 **Type-Safe Generics**: Built using Go Generics (1.18+), ensuring No-Magic and no `interface{}` casting.
-   🛡️ **Stampede Protection**: Built-in `singleflight` integration to collapse concurrent database hits for the same key into a single request.
-   🛠️ **ID Projection (Auto-Repair)**: Advanced `MGet` logic that identifies missing entities, fetches them in a single batch from the database, and automatically repairs the cache.
-   🧩 **Declarative DX**: Register your entities with a "Batch Fetcher" once, and use them anywhere.
-   💾 **Memory Efficient**: Focuses on **Entity-Level Caching** to avoid data redundancy in RAM.

## Installation

```bash
go get github.com/kaine/stampede
```

## Quick Start (Declarative Usage)

```go
import (
    "github.com/kaine/stampede/pkg/stampede"
    "github.com/redis/go-redis/v9"
)

func main() {
    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    reg := stampede.NewRegistry(rdb)

    // 1. Declaratively register your entity
    UserCache := stampede.Register[int, User](reg, "users", func(ctx context.Context, ids []int) (map[int]User, error) {
        return db.Users.FindMany(ids) // Batch fetcher
    })

    // 2. Use it
    user, _ := UserCache.Get(ctx, 1)        // Cache-First + Singleflight
    users, _ := UserCache.MGet(ctx, []int{1, 2, 3}) // Auto-batching repair
}
```

## Architecture

Stampede follows the **Two-Tier Fetching** pattern:
1.  **Tier 1 (DB)**: Handles "Who" to show (ID Projection/Pagination) using indices.
2.  **Tier 2 (Cache)**: Handles "What" to show (Full Payloads) using O(1) Redis lookups.

For more details, see [ARCHITECTURE.md](./ARCHITECTURE.md).

## Testing

Verified with BDD (Gherkin/Godog):
```bash
go test -v ./features/...
```

## License
MIT
