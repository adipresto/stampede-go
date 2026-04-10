package main

import (
	"context"
	"fmt"
	"time"

	"github.com/kaine/stampede/pkg/stampede"
	"github.com/redis/go-redis/v9"
)

// Member is our entity model
type Member struct {
	ID    int
	Email string
}

func main() {
	// 1. Setup Redis Client
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// 2. Create Declarative Registry
	reg := stampede.NewRegistry(rdb)

	// 3. Register Entity Cache (Declarative Definition)
	MemberCache := stampede.Register[int, Member](reg, "members", func(ctx context.Context, ids []int) (map[int]Member, error) {
		fmt.Printf(">> [DB Fetch] Querying for IDs: %v\n", ids)
		
		// Simulate database result
		results := make(map[int]Member)
		for _, id := range ids {
			results[id] = Member{ID: id, Email: fmt.Sprintf("user-%d@example.com", id)}
		}
		return results, nil
	}, stampede.WithTTL(10*time.Minute))

	ctx := context.Background()

	// 4. Usage: Single Get (Aside + Singleflight)
	fmt.Println("--- Single Get ---")
	m1, _ := MemberCache.Get(ctx, 1)
	fmt.Printf("Result: %+v\n", m1)

	// 5. Usage: MGet (Auto-Repair for Pagination/ID Projection)
	fmt.Println("\n--- Multi Get (MGet) ---")
	ids := []int{1, 2, 3, 4, 5}
	members, _ := MemberCache.MGet(ctx, ids)
	for _, m := range members {
		fmt.Printf("Member: %v\n", m)
	}

	// 6. Usage: Sparse Fieldsets (Field Projection)
	fmt.Println("\n--- Sparse Fieldsets ---")
	fields := []string{"Email"} // Only fetch Email
	mFields, _ := MemberCache.GetFields(ctx, 1, fields)
	fmt.Printf("Member 1 (Fields: %v): %v\n", fields, mFields)
}
