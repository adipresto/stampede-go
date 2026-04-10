Feature: Sparse Fieldsets (Field Projection)
  As a developer
  I want to retrieve only specific fields of an entity
  So that I can reduce bandwidth and support mobile/GraphQL-like queries

  Background:
    Given a redis cache is running
    And an entity "users" is registered with a DB fetcher

  Scenario: Requesting a single entity with specific fields
    Given the cache does NOT contain "users:id:100"
    And the database contains "users:id:100" with data '{"ID":100, "Name":"Alice", "Email":"alice@example.com"}'
    When I request entity "users" with ID 100 and fields ["ID", "Name"]
    Then the result should only contain fields ["ID", "Name"]
    And the result should be '{"ID":100, "Name":"Alice"}'
    And the cache should now contain "users:id:100"

  Scenario: Requesting multiple entities with specific fields
    Given the cache contains "users:id:1" with data '{"ID":1, "Name":"User1", "Email":"u1@ex.com"}'
    And the cache contains "users:id:2" with data '{"ID":2, "Name":"User2", "Email":"u2@ex.com"}'
    When I request multiple "users" with IDs [1, 2] and fields ["Name"]
    Then the result should contain 2 entities
    And entity 1 should be '{"Name":"User1"}'
    And entity 2 should be '{"Name":"User2"}'

  Scenario: Requesting empty fields returns all fields
    Given the cache contains "users:id:1" with data '{"ID":1, "Name":"User1"}'
    When I request entity "users" with ID 1 and fields []
    Then the result should be '{"ID":1, "Name":"User1"}'
