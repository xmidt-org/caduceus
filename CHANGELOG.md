# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]
- Fix for webhook update for all fields
- Fix for retry logic so all failures are retried the specified number of times
- Fix for waiting for DNS to resolve prior to listening for webhook updates
- Fix for cpu spike after about 10 mintues due to worker go routines not finishing.

## [0.1.1] - 2018-04-06
### Added
- Fix for X-Webpa-Event header
- Use all cores-1 for IO control by default
- Fix a bug where the regex matching was too greedy
- Add retries on errors deemed "retryable"
- Add all X-Midt headers
- Add metrics data

## [0.0.1] - 2017-03-28
### Added
- Initial creation

[Unreleased]: https://github.com/Comcast/caduceus/compare/0.1.1...HEAD
[0.1.1]: https://github.com/Comcast/caduceus/compare/0.0.1...0.1.1
[0.0.1]: https://github.com/Comcast/caduceus/compare/0.0.0...0.0.1
