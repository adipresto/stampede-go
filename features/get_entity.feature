Feature: Get Entity
  As a developer
  I want to retrieve a single entity by ID
  So that I can serve requests faster using the cache

  Background:
    Given a redis cache is running
    And an entity "users" is registered with a DB fetcher

  Scenario: Getting a single entity when cache is empty (Cache Miss)
    Given the cache does not contain "users:id:1"
    And the database contains "users:id:1" with data '{"ID":1, "Name":"Alice"}'
    When I request entity "users" with ID 1
    Then the result should be '{"ID":1, "Name":"Alice"}'
    And the cache should now contain "users:id:1"

  Scenario: Getting a single entity when cache is populated (Cache Hit)
    Given the cache contains "users:id:1" with data '{"ID":1, "Name":"Alice"}'
    When I request entity "users" with ID 1
    Then the result should be '{"ID":1, "Name":"Alice"}'
    And the database should NOT be called
