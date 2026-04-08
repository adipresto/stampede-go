Feature: Multi-Get Entities (MGet)
  As a developer
  I want to retrieve multiple entities by their IDs in one call
  So that I can implement efficient ID Projection and pagination

  Background:
    Given a redis cache is running
    And an entity "users" is registered with a DB fetcher

  Scenario: Getting multiple entities with partial cache hits (Auto-Repair)
    Given the cache contains "users:id:10" with data '{"ID":10, "Name":"User10"}'
    And the cache does NOT contain "users:id:11"
    And the database contains "users:id:11" with data '{"ID":11, "Name":"User11"}'
    When I request multiple "users" with IDs [10, 11]
    Then the result should contain 2 entities
    And entity 10 should be '{"ID":10, "Name":"User10"}'
    And entity 11 should be '{"ID":11, "Name":"User11"}'
    And the database should only be queried for ID [11]
    And the cache should now contain "users:id:11"
