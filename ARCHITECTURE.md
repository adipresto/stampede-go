# Architecture

This document describes the internal architecture of the **Stampede** package and the patterns it implements.

## 1. Core Philosophy: Entity-Level Caching

Traditionally, many developers cache the entire API response based on the request URL (e.g., `?page=1&limit=10`). While simple, this leads to **Memory Redundancy** (duplicate objects across different pages) and **Invalidation Nightmares**.

**Stampede** implements **Entity-Level Caching**:
-   **One Key per ID**: Data is stored as `type:id:{ID}`.
-   **Atomicity**: Each entity is fetched and cached independently.
-   **Zero Duplication**: An entity is stored exactly once in Redis, regardless of how many pagination results it appears in.

## 2. Two-Tier Fetching (ID Projection)

To handle complex queries like pagination or filtering efficiently, Stampede uses a two-tier approach:

### Tier 1: ID Discovery (DB)
When a user requests a paginated list, the application should query the database only for IDs:
```sql
SELECT id FROM users WHERE status = 'active' ORDER BY created_at LIMIT 10 OFFSET 50;
```
This is an **Index-Only Scan**, which is extremely fast and light on database I/O.

### Tier 2: Payload Hydration (Stampede MGet)
The list of IDs is passed to Stampede's `MGet` method. Stampede then:
1.  Performs a single `MGET` call to Redis.
2.  Identifies which IDs are missing from the cache.
3.  Calls the registered **Batch Fetcher** only for the missing IDs.
4.  Pipelined-sets the missing entities back into Redis.
5.  Reassembles the final list in the correct order.

## 3. Stampede Protection (Singleflight)

A **Cache Stampede** (or Thundering Herd) occurs when a hot cache key expires and a spike of concurrent requests hits the database simultaneously.

Stampede prevents this using `golang.org/x/sync/singleflight`:
-   When a `Get(id)` call results in a cache miss, it enters a `singleflight` group keyed by the ID.
-   Parallel requests for the same ID wait for the first request to complete.
-   Once the first request fetches the data from the DB and populates the cache, all waiting requests receive the same result without hitting the DB again.

## 4. Declarative Registry

The API is designed to be declarative to improve code maintainability:
-   **Registry**: Manages the connection to Redis.
-   **Register**: Binds a Type, a Prefix, and a Fetcher together. This definition happens once (usually in `init` or `main`), keeping the business logic clean of caching orchestration.

## 5. Generic Type Safety

By using Go Generics, Stampede ensures that:
-   Compilers catch type mismatches.
-   There is no performance overhead from `interface{}` casting or reflection during runtime retrieval.

## 6. Verified Integration (Testability)

Stampede is built with testability in mind. It includes a comprehensive BDD suite that verifies its behavior not just in isolation, but also when integrated with a real `net/http` server. This ensures that the protection against Thundering Herds works correctly across the boundary of an actual web application handler.
