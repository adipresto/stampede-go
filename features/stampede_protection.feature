Feature: Stampede Protection (Singleflight)
  As a developer
  I want to prevent multiple simultaneous DB queries for the same missing key
  So that I can protect my database from the Thundering Herd problem

  Background:
    Given a redis cache is running
    And an entity "users" is registered with a DB fetcher that takes 500ms (slow DB)

  Scenario: Concurrent requests for the same non-existent key
    Given the cache does NOT contain "users:id:99"
    And the database contains "users:id:99"
    When 50 concurrent requests ask for entity "users" with ID 99
    Then all 50 requests should receive '{"ID":99, "Name":"StampedeUser"}'
    And the database fetcher should only have been called EXACTLY 1 time
