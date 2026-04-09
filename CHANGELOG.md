# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2026-04-09

### Added
- **HTTP Server Integration**: Added full BDD verification for `net/http` server usage.
- **Robust BDD Assertions**: Replaced placeholder test steps with real cache, DB, and HTTP response validation.
- **Thread-safe Testing**: Implemented concurrent result capturing for more accurate high-load simulation in tests.

### Fixed
- Fixed Godog `After` hook signatures for version 0.15.1.
- Fixed invalid JSON strings in test scenarios.
- Fixed unused `log` import in the example code.

## [0.1.0] - 2026-04-08

### Added
- Initial release of the **Stampede** package.
- **Declarative Registry**: Support for central entity management via `NewRegistry` and `Register`.
- **Generic Support**: Fully type-safe API for Get and MGet.
- **Singleflight Integration**: Automated protection against concurrent database hits on cache misses.
- **MGet Auto-Repair**: Automated logic to fetch partial cache misses from DB in a single batch.
- **BDD Testing**: Full testing suite covering Get, MGet, and concurrency scenarios.
- **Example**: Basic usage example in `examples/basic`.
