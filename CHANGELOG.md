# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.3] - 2019-02-27
### Changed
- Fix for [issue 126](https://github.com/Comcast/caduceus/issues/126)

## [0.1.2] - 2019-02-21
### Added
- Fix for delivering events as json or msgpack based events [issue 113](https://github.com/Comcast/caduceus/issues/113)

### Changed
- Updated to new version of webpa-common library
- Remove the worker pool as a fixed number of workers per endpoint and simply cap
  the maximum number.  Partial fix for [issue 115](https://github.com/Comcast/caduceus/issues/115), [issue 103](https://github.com/Comcast/caduceus/issues/103)
- Fix for webhook shallow copy bug.  Partial fix for [issue 115](https://github.com/Comcast/caduceus/issues/115)
- Fix for webhook update for all fields (updated webpa-common code to bring in fix)
- Fix for retry logic so all failures are retried the specified number of times - [issue 91](https://github.com/Comcast/caduceus/issues/91)
- Fix for waiting for DNS to resolve prior to listening for webhook updates - [issue 111](https://github.com/Comcast/caduceus/issues/111)
- Fix for cpu spike after about 10 mintues due to worker go routines not finishing.
- Fix logic for updating webhooks
- Fix for sending the same event multiple times to the same webhook.
- Fix for [issue 99](https://github.com/Comcast/caduceus/issues/99)

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

[Unreleased]: https://github.com/Comcast/caduceus/compare/0.1.3...HEAD
[0.1.3]: https://github.com/Comcast/caduceus/compare/0.1.2...0.1.3
[0.1.2]: https://github.com/Comcast/caduceus/compare/0.1.1...0.1.2
[0.1.1]: https://github.com/Comcast/caduceus/compare/0.0.1...0.1.1
[0.0.1]: https://github.com/Comcast/caduceus/compare/0.0.0...0.0.1
