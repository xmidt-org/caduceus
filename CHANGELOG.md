# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.2.3]
- updated release pipeline to use travis [#174](https://github.com/xmidt-org/caduceus/pull/174)
- bumped wrp-go to v2 [#182](https://github.com/xmidt-org/caduceus/pull/182)

## [v0.2.2]
- bump webpa-common to v1.5.0

## [v0.2.1]
### Changed
- Fixed a missing cardinality dimension in a metric that caused a panic.

## [v0.2.0]
### Added
- Metrics to support debugging the problem found by GH Issue [issue 145](https://github.com/Comcast/caduceus/issues/145)
- Add WRP Header support for Partner-Ids and Metadata

### Changed
- converting glide to go mod

## [v0.1.5]
fixed build upload

## [v0.1.4]
### Added
- Add alternative urls and consumer max retry logic for webhooks

### Changed
- Retry on non 2xx status codes
- Fix for no retries being attempted
- Add metric for incoming content type

## [v0.1.3] - 2019-02-27
### Changed
- Fix for [issue 126](https://github.com/Comcast/caduceus/issues/126)

## [v0.1.2] - 2019-02-21
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

[Unreleased]: https://github.com/Comcast/caduceus/compare/v0.2.3...HEAD
[v0.2.3]: https://github.com/Comcast/caduceus/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/Comcast/caduceus/compare/v0.2.2-rc.1...v0.2.2
[v0.2.1]: https://github.com/Comcast/caduceus/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/Comcast/caduceus/compare/v0.1.5...v0.2.0
[v0.1.5]: https://github.com/Comcast/caduceus/compare/v0.1.4...v0.1.5
[v0.1.4]: https://github.com/Comcast/caduceus/compare/v0.1.3...v0.1.4
[v0.1.3]: https://github.com/Comcast/caduceus/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/Comcast/caduceus/compare/0.1.1...v0.1.2
[0.1.1]: https://github.com/Comcast/caduceus/compare/0.0.1...0.1.1
[0.0.1]: https://github.com/Comcast/caduceus/compare/v0.0.0...0.0.1
