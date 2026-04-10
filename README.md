# Stampede

**Stampede** is a developer-friendly Redis wrapper for Go, specifically designed to solve the **Thundering Herd (Cache Stampede)** problem and optimize **ID Projection** patterns in massive datasets.

## Key Features

-   🚀 **Type-Safe Generics**: Built using Go Generics (1.18+), ensuring No-Magic and no `interface{}` casting.
-   🛡️ **Stampede Protection (Level 1 & 2)**: 
    - **L1**: Singleflight integration for single key lookups.
    - **L2 (DataLoader)**: Global Batcher that merges independent `MGet` requests from different users into a single unified database query.
-   🛠️ **ID Projection (Auto-Repair)**: Advanced `MGet` logic that identifies missing entities, fetches them in a single batch, and automatically repairs the cache.
-   🎯 **Sparse Fieldsets**: Retrieve only specific fields from a cached entity (Field Projection) to reduce bandwidth and support mobile-optimized responses.
-   🧩 **Declarative DX**: Register your entities with a "Batch Fetcher" once, and use them anywhere.
-   💾 **Memory Efficient**: Focuses on **Entity-Level Caching** (1 Key = 1 Row) to eliminate data redundancy in RAM.

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

    // 1. Declaratively register your entity with Global Batching (Level 2)
    UserCache := stampede.Register[int, User](reg, "users", func(ctx context.Context, ids []int) (map[int]User, error) {
        return db.Users.FindMany(ids) // All concurrent requests will be merged here!
    }, stampede.WithBatching(5 * time.Millisecond, 100))

    // 2. Simple Usage
    user, _ := UserCache.Get(ctx, 1)        // Cache-First + Singleflight (L1)
    users, _ := UserCache.MGet(ctx, []int{1, 2, 3}) // Global Batching (L2)

    // 3. Sparse Fieldsets (Field Projection)
    // Only fetch "Name" and "Email" from the cached "User" entity.
    partial, _ := UserCache.GetFields(ctx, 1, []string{"Name", "Email"}) 
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
