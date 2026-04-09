Feature: HTTP Server Integration
  As a developer
  I want to use Stampede within a real HTTP server
  So that I can protect my database from concurrent web traffic

  Background:
    Given a redis cache is running
    And an entity "users" is registered with a DB fetcher that takes 500ms (slow DB)
    And a net/http server is running with a Stampede-enabled endpoint "/users/{id}"

  Scenario: Concurrent HTTP requests to a protected endpoint
    And the database contains "users:id:55"
    When 20 concurrent HTTP GET requests are made to "/users/55"
    Then all 20 HTTP responses should have status 200
    And all 20 HTTP responses should contain '{"ID":55, "Name":"StampedeUser"}'
    And the database fetcher should only have been called EXACTLY 1 time
