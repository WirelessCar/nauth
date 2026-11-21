# Changelog

## 0.8.1 (2026-11-21)

<!-- Release notes generated using configuration in .github/release.yml at main -->

## What's Changed
### Other Changes
* chore(deps): update dependency node to v26 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/442
* chore(deps): update jdx/mise-action action to v5 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/446
* chore(deps): update actions/checkout action to v7 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/437
* chore(deps): update actions/github-script action to v9 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/438
* chore(deps): update actions/setup-go action to v7 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/439
* chore(deps): update actions/setup-node action to v7 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/440
* chore(deps): update actions/upload-artifact action to v7 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/441
* chore(deps): update actions/cache action to v6 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/436
* chore(deps): update dependency unplugin-icons to v24 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/444
* chore(deps): update helm release kube-prometheus-stack to v93 by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/445
* chore(deps): update minor dependency updates by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/432
* chore(deps): update security vulnerability updates [security] by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/431
* chore(renovate): enable patch updates and automatic major PRs by @choufraise in https://github.com/WirelessCar/nauth/pull/447
* fix(ci): stabilize release labeling and allow its trusted workflow by @choufraise in https://github.com/WirelessCar/nauth/pull/450
* fix(deps): resolve NATS security alerts and restore Renovate updates by @choufraise in https://github.com/WirelessCar/nauth/pull/451
* feat(fips): add FIPS 140-3 operator image by @mkm29 in https://github.com/WirelessCar/nauth/pull/434

## New Contributors
* @mkm29 made their first contribution in https://github.com/WirelessCar/nauth/pull/434

**Full Changelog**: https://github.com/WirelessCar/nauth/compare/v0.8.0...v0.8.1

## 0.8.0 (2026-11-06)

<!-- Release notes generated using configuration in .github/release.yml at main -->

## What's Changed
### Breaking Changes ⚠️
* fix(api)!: preserve omitted Account and User limit fields by @choufraise in https://github.com/WirelessCar/nauth/pull/406
* feat(jetstream)!: stop implicitly enabling JetStream by @choufraise in https://github.com/WirelessCar/nauth/pull/418
### Other Changes
* ci: prepare workflows for merge queues by @choufraise in https://github.com/WirelessCar/nauth/pull/395
* chore(ci): fail CI on untidy go.mod by @wingsofovnia in https://github.com/WirelessCar/nauth/pull/393
* chore(deps): update security vulnerability updates [security] by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/386
* docs: clarify contribution requirements by @choufraise in https://github.com/WirelessCar/nauth/pull/396
* ci: add automatic PR number suffixes (#397) by @choufraise in https://github.com/WirelessCar/nauth/pull/397
* ci: use the workflow token for PR title updates by @choufraise in https://github.com/WirelessCar/nauth/pull/398
* ci: remove obsolete rebase check (#399) by @choufraise in https://github.com/WirelessCar/nauth/pull/399
* ci(pr): stop adding PR numbers to titles by @choufraise in https://github.com/WirelessCar/nauth/pull/400
* fix(e2e-ctl): handle expected port-forward shutdown errors by @choufraise in https://github.com/WirelessCar/nauth/pull/401
* chore(deps): update minor dependency updates by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/379
* feat(release): automate stable and prerelease releases by @choufraise in https://github.com/WirelessCar/nauth/pull/408
* chore(deps): update module golang.org/x/net to v0.62.0 [security] by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/405
* chore(deps): update minor dependency updates by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/404
* ci(release): highlight breaking changes in PR-based release notes by @choufraise in https://github.com/WirelessCar/nauth/pull/410
* fix(user): preserve User identity across reconciliations by @choufraise in https://github.com/WirelessCar/nauth/pull/411
* feat(chart): support imagePullSecrets for the operator Pod by @pkieszcz in https://github.com/WirelessCar/nauth/pull/414
* feat(user): configure credentials Secret data key by @choufraise in https://github.com/WirelessCar/nauth/pull/415
* fix(ci): restore labeling for older merged pull requests by @choufraise in https://github.com/WirelessCar/nauth/pull/416
* docs(migrations): add NAuth upgrade guide by @choufraise in https://github.com/WirelessCar/nauth/pull/417
* chore(deps): update minor dependency updates by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/412
* ci(release): accept Release Please's initial RC tag by @choufraise in https://github.com/WirelessCar/nauth/pull/420
* chore(release): promote verified 0.8.0-rc to main by @choufraise in https://github.com/WirelessCar/nauth/pull/426
* ci(e2e): report required checks for merge queue by @choufraise in https://github.com/WirelessCar/nauth/pull/428
* chore(deps): update module github.com/klauspost/compress to v1.20.2 [security] by @renovate[bot] in https://github.com/WirelessCar/nauth/pull/427
* fix(release): restore the stable release baseline by @choufraise in https://github.com/WirelessCar/nauth/pull/429
* ci(release): publish manually created release candidates by @choufraise in https://github.com/WirelessCar/nauth/pull/430

## New Contributors
* @pkieszcz made their first contribution in https://github.com/WirelessCar/nauth/pull/414

**Full Changelog**: https://github.com/WirelessCar/nauth/compare/v0.7.8...v0.8.0
