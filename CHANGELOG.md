## [1.17.3](https://github.com/mogenius/renovate-operator/compare/1.17.2...1.17.3) (2026-01-22)


### Bug Fixes

* switched labels and annotations for grafana dashboard ([62731dc](https://github.com/mogenius/renovate-operator/commit/62731dc632f920903e7d9bdbd90a573aae637cd5))

## [1.17.2](https://github.com/mogenius/renovate-operator/compare/1.17.1...1.17.2) (2026-01-22)


### Bug Fixes

* adding missing grafana dashboard ([191ca05](https://github.com/mogenius/renovate-operator/commit/191ca05fba0043594f82ce40a81f3002161e94b3))

## [1.17.1](https://github.com/mogenius/renovate-operator/compare/1.17.0...1.17.1) (2026-01-16)


### Bug Fixes

* rename labels to not colide with default prometheus labels ([a859693](https://github.com/mogenius/renovate-operator/commit/a8596936b1c51b424057a5a4422867a8d4735f3d))

# [1.17.0](https://github.com/mogenius/renovate-operator/compare/1.16.2...1.17.0) (2026-01-16)


### Features

* adding basic infrastructure for adding metrics ([e7e6300](https://github.com/mogenius/renovate-operator/commit/e7e6300145b4615de1d80cb4ce15eb99d14c1ac5))

## [1.16.2](https://github.com/mogenius/renovate-operator/compare/1.16.1...1.16.2) (2026-01-13)


### Bug Fixes

* allow the operator to read webhook integration tokens ([15efd4c](https://github.com/mogenius/renovate-operator/commit/15efd4ce84065d06aa8a901db8e7c02e33ef2166))

## [1.16.1](https://github.com/mogenius/renovate-operator/compare/1.16.0...1.16.1) (2026-01-13)


### Bug Fixes

* point webhook route and ingress to the right service ([85a26f9](https://github.com/mogenius/renovate-operator/commit/85a26f94d01f9146785965e395a3cadf2487eba6))

# [1.16.0](https://github.com/mogenius/renovate-operator/compare/1.15.0...1.16.0) (2026-01-13)


### Features

* adding provider specific webhooks ([f2db773](https://github.com/mogenius/renovate-operator/commit/f2db773b10b085d50b73e4352ad26d40569993bd))

# [1.15.0](https://github.com/mogenius/renovate-operator/compare/1.14.0...1.15.0) (2026-01-05)


### Features

* **ui:** add dark theme ([0bdb4c0](https://github.com/mogenius/renovate-operator/commit/0bdb4c03bb9fef00227226bd05d5638a553fc5d7))

# [1.14.0](https://github.com/mogenius/renovate-operator/compare/1.13.2...1.14.0) (2026-01-05)


### Features

* adding gateway api support ([5804f9b](https://github.com/mogenius/renovate-operator/commit/5804f9bd345aa8726d91a48a683659fc76239094))

## [1.13.2](https://github.com/mogenius/renovate-operator/compare/1.13.1...1.13.2) (2026-01-04)


### Bug Fixes

* **deps:** update k8s.io/utils digest to 718f0e5 ([6fd3b18](https://github.com/mogenius/renovate-operator/commit/6fd3b181070a4b916b334b005dc32d34d59c0c8f))

## [1.13.1](https://github.com/mogenius/renovate-operator/compare/1.13.0...1.13.1) (2026-01-04)


### Bug Fixes

* **deps:** update kubernetes packages to v0.35.0 ([c5d1f6f](https://github.com/mogenius/renovate-operator/commit/c5d1f6ff528fe8824695e84e963e76b1888860cc))

# [1.13.0](https://github.com/mogenius/renovate-operator/compare/1.12.0...1.13.0) (2025-12-23)


### Features

* adding sorting to columns ([7eed40c](https://github.com/mogenius/renovate-operator/commit/7eed40caf48d446fbf77dbbfa529dc47b4657455))

# [1.12.0](https://github.com/mogenius/renovate-operator/compare/1.11.0...1.12.0) (2025-12-13)


### Features

* package local dependencies instead of relying on CDNs ([167c6a1](https://github.com/mogenius/renovate-operator/commit/167c6a16dc0020f98cb76a8888098dd001f929bc))

# [1.11.0](https://github.com/mogenius/renovate-operator/compare/1.10.1...1.11.0) (2025-12-13)


### Features

* watch namespaces only ([225047a](https://github.com/mogenius/renovate-operator/commit/225047ae286a9334f9d84d82b10265c3addcac7c))

## [1.10.1](https://github.com/mogenius/renovate-operator/compare/1.10.0...1.10.1) (2025-12-11)


### Bug Fixes

* **deps:** update kubernetes packages to v0.34.3 ([575fcf4](https://github.com/mogenius/renovate-operator/commit/575fcf4fdceaadd4ab54c9d90650490d80b0fcb2))

# [1.10.0](https://github.com/mogenius/renovate-operator/compare/1.9.0...1.10.0) (2025-12-08)


### Features

* add OCI registry support with cosign signing for Helm charts ([af667b6](https://github.com/mogenius/renovate-operator/commit/af667b68fe15fd1e76a62b15a1d1f7a9e739467a)), closes [#52](https://github.com/mogenius/renovate-operator/issues/52)

# [1.9.0](https://github.com/mogenius/renovate-operator/compare/1.8.1...1.9.0) (2025-12-04)


### Features

* adding more flexible scheduling options ([d4e2a67](https://github.com/mogenius/renovate-operator/commit/d4e2a674dd51b1c9d87f0ff5ab679216259f2eec))

## [1.8.1](https://github.com/mogenius/renovate-operator/compare/1.8.0...1.8.1) (2025-12-04)


### Bug Fixes

* trim job names and add a hash ([dc68db3](https://github.com/mogenius/renovate-operator/commit/dc68db3c15cb12aaf7029339b00dd16905dd5ca7))

# [1.8.0](https://github.com/mogenius/renovate-operator/compare/1.7.9...1.8.0) (2025-12-03)


### Features

* adding the ability to add extra volumes ([71a4c5d](https://github.com/mogenius/renovate-operator/commit/71a4c5dab7f3153fb231033ba2433efc3d0bc3e8))

## [1.7.9](https://github.com/mogenius/renovate-operator/compare/1.7.8...1.7.9) (2025-12-03)


### Bug Fixes

* responsive accordion header layout with flex-wrap and removes second btn spinner ([cb3ed50](https://github.com/mogenius/renovate-operator/commit/cb3ed5027e06329365d8b9327c241212a3485836))

## [1.7.8](https://github.com/mogenius/renovate-operator/compare/1.7.7...1.7.8) (2025-11-25)


### Bug Fixes

* ran go mod tidy ([6deff25](https://github.com/mogenius/renovate-operator/commit/6deff2545a1d0c5a443434a2beec24957ffa0989))

## [1.7.7](https://github.com/mogenius/renovate-operator/compare/1.7.6...1.7.7) (2025-11-21)


### Bug Fixes

* **deps:** update kubernetes packages to v0.34.2 ([cbc5e10](https://github.com/mogenius/renovate-operator/commit/cbc5e10ff1603569d9e07275487311c8f92c8011))

## [1.7.6](https://github.com/mogenius/renovate-operator/compare/1.7.5...1.7.6) (2025-11-21)


### Bug Fixes

* **deps:** update module sigs.k8s.io/controller-runtime to v0.22.4 ([ca092f6](https://github.com/mogenius/renovate-operator/commit/ca092f64f5654d2267b003420ed5f34e6a31a92f))

## [1.7.5](https://github.com/mogenius/renovate-operator/compare/1.7.4...1.7.5) (2025-11-19)


### Bug Fixes

* add missing discoveryStatus state and polling ([80c4d2a](https://github.com/mogenius/renovate-operator/commit/80c4d2a5a787fc6a1de6425d7c4d04f53659bffa))

## [1.7.4](https://github.com/mogenius/renovate-operator/compare/1.7.3...1.7.4) (2025-11-18)


### Bug Fixes

* compile error and discovery ([9cdce47](https://github.com/mogenius/renovate-operator/commit/9cdce4761b3c24752716349da86f361d897c7844))

## [1.7.3](https://github.com/mogenius/renovate-operator/compare/1.7.2...1.7.3) (2025-11-18)


### Bug Fixes

* wait for the discovery job to appear ([b7b008c](https://github.com/mogenius/renovate-operator/commit/b7b008cca170427c3de56c2e90c62c51ca509925))

## [1.7.2](https://github.com/mogenius/renovate-operator/compare/1.7.1...1.7.2) (2025-11-18)


### Bug Fixes

* wait for discoveryjob deletion ([a797a8b](https://github.com/mogenius/renovate-operator/commit/a797a8b9a9f3e46b7a7a7bd4df5a3f640b7ab21c))

## [1.7.1](https://github.com/mogenius/renovate-operator/compare/1.7.0...1.7.1) (2025-11-18)


### Bug Fixes

* remove unnecessary load for discovery job ([91afdc5](https://github.com/mogenius/renovate-operator/commit/91afdc5dd2cc2cc2a8b0e3f8138f8e149e37859a))

# [1.7.0](https://github.com/mogenius/renovate-operator/compare/1.6.9...1.7.0) (2025-11-18)


### Bug Fixes

* adapt tests to reflect new api behaviour ([4877ff5](https://github.com/mogenius/renovate-operator/commit/4877ff5eae61a678698ae19b76345dd85267c53c))
* add cronExpression to renovatejobs api ([577240e](https://github.com/mogenius/renovate-operator/commit/577240e0d7f896a0578d502d652f7b4c8cbd3159))
* also apply restrictions to trigger to mobile view ([e2cab77](https://github.com/mogenius/renovate-operator/commit/e2cab773e20b004c653b6e55c09707c0617ab453))
* block triggering already scheduled projects ([fb4677a](https://github.com/mogenius/renovate-operator/commit/fb4677a414ee9ccc1f669f7bf1c6b20f6b4535f6))
* deactivate buttons for running jobs ([59f38e8](https://github.com/mogenius/renovate-operator/commit/59f38e89e8cebbd1584478d53ed5816445a0bc8e))


### Features

* modernize UI with logo, live countdown, and layout stability fixes ([8343397](https://github.com/mogenius/renovate-operator/commit/8343397e8654da581a33e8b6a4b6107ecf73eef8))

## [1.6.9](https://github.com/mogenius/renovate-operator/compare/1.6.8...1.6.9) (2025-11-14)


### Bug Fixes

* added debug config for vscode ([a829862](https://github.com/mogenius/renovate-operator/commit/a829862a4e7ebd9d7e83863e0de8e6db7e90e770))
* remove retry function ([5113880](https://github.com/mogenius/renovate-operator/commit/51138807990c4b016b7b72196906a5ef3c8465aa))
* updated readme ([73a3460](https://github.com/mogenius/renovate-operator/commit/73a3460f5adbb79d7ae1def0cde8c2a75767b47a))

## [1.6.8](https://github.com/mogenius/renovate-operator/compare/1.6.7...1.6.8) (2025-11-14)


### Bug Fixes

* add args for os and arch ([a5ba29a](https://github.com/mogenius/renovate-operator/commit/a5ba29a020ee12bb63444df349ae50019ad1dcac))
* use github app to publish new releases ([916107f](https://github.com/mogenius/renovate-operator/commit/916107f87bf5a0a94df43beaf620703cc16dbd12))

## [1.6.7](https://github.com/mogenius/renovate-operator/compare/1.6.6...1.6.7) (2025-11-14)


### Bug Fixes

* new multi-arch workflow ([00404f8](https://github.com/mogenius/renovate-operator/commit/00404f8e7451c6a704aa5465af8aa426b524091f))

## [1.6.6](https://github.com/mogenius/renovate-operator/compare/1.6.5...1.6.6) (2025-11-13)


### Bug Fixes

* added justfile to make development more convenient ([032a8cb](https://github.com/mogenius/renovate-operator/commit/032a8cb3b9eae5dda53eb4eff8576757c77c516b))

## [1.6.5](https://github.com/mogenius/renovate-operator/compare/1.6.4...1.6.5) (2025-11-13)


### Bug Fixes

* reworks ui-layout ([aca7e68](https://github.com/mogenius/renovate-operator/commit/aca7e686330525ffa6a8637ec75f595e0da77f22))

## [1.6.4](https://github.com/mogenius/renovate-operator/compare/1.6.3...1.6.4) (2025-11-12)


### Bug Fixes

* cannot set failed project to be scheduled ([b8bd04c](https://github.com/mogenius/renovate-operator/commit/b8bd04c22f69b727a8b9f15d8f7a70935a296d0d))

## [1.6.3](https://github.com/mogenius/renovate-operator/compare/1.6.2...1.6.3) (2025-11-12)


### Bug Fixes

* adding nextSchedule to renovatejobs request ([89bcace](https://github.com/mogenius/renovate-operator/commit/89bcace5da1cf7c8fad95e7b077faede33eb83c4))
* adding settings for backofflimits ([1fb498b](https://github.com/mogenius/renovate-operator/commit/1fb498bb16ac0e4f58f2a61cccbe0a10c2b6f79b))
* allow setting value wether successfull jobs should be deleted ([b0d91d7](https://github.com/mogenius/renovate-operator/commit/b0d91d76fd35abd101753424370159e98816ee54))
* go test and golangci lint ([64e0791](https://github.com/mogenius/renovate-operator/commit/64e079136769d60611e09f4a8fb18016a21ac798))
* if the job cannot be found it is considered failed ([8ff2558](https://github.com/mogenius/renovate-operator/commit/8ff255875250f9390b0298ea56d67619570c173b))
* lastRun will now be propagated ([bcf26cb](https://github.com/mogenius/renovate-operator/commit/bcf26cb1cd0a5458267b852c89fa7371acff484b))
* pass time.time and not kubernetes time ([cda7635](https://github.com/mogenius/renovate-operator/commit/cda76357a5b00833ce2dc95c4b474579e98a7c77))

## [1.6.2](https://github.com/mogenius/renovate-operator/compare/1.6.1...1.6.2) (2025-11-12)


### Bug Fixes

* do not create job names with . ([8885ee6](https://github.com/mogenius/renovate-operator/commit/8885ee6ca79ea868a9baa46516e90bfcf4d2cdb8))

## [1.6.1](https://github.com/mogenius/renovate-operator/compare/1.6.0...1.6.1) (2025-11-12)


### Bug Fixes

* do not fail to start discovery if no discovery exists ([32c46e0](https://github.com/mogenius/renovate-operator/commit/32c46e0a5eb70dc0b0f71117c0cad00da06ca16b))

# [1.6.0](https://github.com/mogenius/renovate-operator/compare/1.5.1...1.6.0) (2025-11-12)


### Features

* adding the ability to set autodiscovery topics ([6cb940e](https://github.com/mogenius/renovate-operator/commit/6cb940eda47c7f06d17fd04f5741961c7fc59c1c))

## [1.5.1](https://github.com/mogenius/renovate-operator/compare/1.5.0...1.5.1) (2025-11-12)


### Bug Fixes

* adding next run back to health check ([4d28a91](https://github.com/mogenius/renovate-operator/commit/4d28a910c8d666bc686ce8421fceb6b88e23234d))
* do not print node warnings in discovery to retain parsability ([b7e6993](https://github.com/mogenius/renovate-operator/commit/b7e6993dca1ede5b95f5846661b83ab984fd2a60))

# [1.5.0](https://github.com/mogenius/renovate-operator/compare/1.4.1...1.5.0) (2025-11-10)


### Features

* adding webhook api with authentication ([db9bd07](https://github.com/mogenius/renovate-operator/commit/db9bd079b904ecda43d77b302f48d25ce590debd))

## [1.4.1](https://github.com/mogenius/renovate-operator/compare/1.4.0...1.4.1) (2025-11-07)


### Bug Fixes

* **deps:** update dependency go to v1.25.4 ([acbff09](https://github.com/mogenius/renovate-operator/commit/acbff09cd306110aa159140d1ce17ed6ee713e15))

# [1.4.0](https://github.com/mogenius/renovate-operator/compare/1.3.4...1.4.0) (2025-10-31)


### Bug Fixes

* **deps:** update k8s.io/utils digest to bc988d5 ([57f0071](https://github.com/mogenius/renovate-operator/commit/57f00719cb32f63b3aeee052cade9fb782e52bc1))
* remove the need to overwrite sleep function for tests ([62e0186](https://github.com/mogenius/renovate-operator/commit/62e01861ca71f98907505ebea23915aa40e75532))


### Features

* smoothing ui updates and removing flicker ([becf09d](https://github.com/mogenius/renovate-operator/commit/becf09db357f5b026575599e08ad1a1e2df607dd))
* validate status changes for projects ([cbb2843](https://github.com/mogenius/renovate-operator/commit/cbb2843c73ed4421a5ef1e09c007db6e14796049))

## [1.3.4](https://github.com/mogenius/renovate-operator/compare/1.3.3...1.3.4) (2025-10-31)


### Bug Fixes

* adding golang tests ([54621a7](https://github.com/mogenius/renovate-operator/commit/54621a7c0eb4bcb563bd7965bd52f806803d2703))

## [1.3.3](https://github.com/mogenius/renovate-operator/compare/1.3.2...1.3.3) (2025-10-27)


### Bug Fixes

* **deps:** update module sigs.k8s.io/controller-runtime to v0.22.3 ([80ed989](https://github.com/mogenius/renovate-operator/commit/80ed989fa647eb0546b4f0fc14b1379044090339))

## [1.3.2](https://github.com/mogenius/renovate-operator/compare/1.3.1...1.3.2) (2025-10-19)


### Bug Fixes

* **deps:** update dependency go to v1.25.3 ([2efa2df](https://github.com/mogenius/renovate-operator/commit/2efa2df2df62900c7d52c325e41397a41d05bae5))

## [1.3.1](https://github.com/mogenius/renovate-operator/compare/1.3.0...1.3.1) (2025-09-25)


### Bug Fixes

* a few little ui refinements ([e75c86e](https://github.com/mogenius/renovate-operator/commit/e75c86ec4c1a71419067339d0d93eb4516fa40ca))

# [1.3.0](https://github.com/mogenius/renovate-operator/compare/1.2.8...1.3.0) (2025-09-19)


### Features

* applying naming best practices to the helm chart ([4c6aba7](https://github.com/mogenius/renovate-operator/commit/4c6aba781eb692e7286ff2964395ce95438efedc))

## [1.2.8](https://github.com/mogenius/renovate-operator/compare/1.2.7...1.2.8) (2025-09-18)


### Bug Fixes

* **deps:** update k8s.io/utils digest to 0af2bda ([7a1b3dd](https://github.com/mogenius/renovate-operator/commit/7a1b3dded69008a6d8ac4ad0bcd260db484e2eaf))

## [1.2.7](https://github.com/mogenius/renovate-operator/compare/1.2.6...1.2.7) (2025-09-18)


### Bug Fixes

* **deps:** update module github.com/go-logr/logr to v1.4.3 ([f6e74d5](https://github.com/mogenius/renovate-operator/commit/f6e74d5edcf8332290e6e4e37fc169e5780d14d7))

## [1.2.6](https://github.com/mogenius/renovate-operator/compare/1.2.5...1.2.6) (2025-09-18)


### Bug Fixes

* **deps:** update kubernetes packages to v0.34.1 ([67d79ab](https://github.com/mogenius/renovate-operator/commit/67d79ab2ff37adf618361d314a67346dd779399f))

## [1.2.5](https://github.com/mogenius/renovate-operator/compare/1.2.4...1.2.5) (2025-09-18)


### Bug Fixes

* **deps:** update module sigs.k8s.io/controller-runtime to v0.22.1 ([2ddacb1](https://github.com/mogenius/renovate-operator/commit/2ddacb115ecffaac41fa69b7a62d3ad5b21b012b))

## [1.2.4](https://github.com/mogenius/renovate-operator/compare/1.2.3...1.2.4) (2025-09-17)


### Bug Fixes

* **deps:** update golang docker tag to v1.25 ([#9](https://github.com/mogenius/renovate-operator/issues/9)) ([ebb0fb3](https://github.com/mogenius/renovate-operator/commit/ebb0fb36f2c8f1e4577f10a2913ac98045f88e52))

## [1.2.3](https://github.com/mogenius/renovate-operator/compare/1.2.2...1.2.3) (2025-09-16)


### Bug Fixes

* display errors in discovery jobs ([ffe460f](https://github.com/mogenius/renovate-operator/commit/ffe460f6cf7e5617dc1fe0a7ec4fda589216f5de))

## [1.2.2](https://github.com/mogenius/renovate-operator/compare/1.2.1...1.2.2) (2025-09-15)


### Bug Fixes

* do not return error on discovery job not found ([bd36089](https://github.com/mogenius/renovate-operator/commit/bd36089d98d3d4f2f0091ae3950ebd74ef92d318))

## [1.2.1](https://github.com/mogenius/renovate-operator/compare/1.2.0...1.2.1) (2025-09-15)


### Bug Fixes

* improve http error messages ([5f903df](https://github.com/mogenius/renovate-operator/commit/5f903dfdc7a575752192aa6c755acec8cae1f92c))

# [1.2.0](https://github.com/mogenius/renovate-operator/compare/1.1.0...1.2.0) (2025-09-15)


### Features

* adding a discovery button to the ui ([dd4db47](https://github.com/mogenius/renovate-operator/commit/dd4db47fa467870b36dbb16e1db72fc65b7c1e3a))

# [1.1.0](https://github.com/mogenius/renovate-operator/compare/1.0.7...1.1.0) (2025-09-15)


### Bug Fixes

* semantic release token ([df2b092](https://github.com/mogenius/renovate-operator/commit/df2b09204c68a76ae4ea67d9069a8887a721edbd))


### Features

* adding default timeout of 30min ([d34d049](https://github.com/mogenius/renovate-operator/commit/d34d0495a6651d08de28cd02c2fa753c9cb092ec))

## [1.0.7](https://github.com/mogenius/renovate-operator/compare/1.0.6...1.0.7) (2025-09-10)


### Bug Fixes

* first commit and then rebase ([a1fe4b0](https://github.com/mogenius/renovate-operator/commit/a1fe4b09bdb4c27d9d791f3eac6f5c5b7535b80a))

## [1.0.6](https://github.com/mogenius/renovate-operator/compare/1.0.5...1.0.6) (2025-09-10)


### Bug Fixes

* pull before commiting changes ([64958cb](https://github.com/mogenius/renovate-operator/commit/64958cb000e30ca1fd49c4ce2faeb15571fe46f5))

## [1.0.5](https://github.com/mogenius/renovate-operator/compare/1.0.4...1.0.5) (2025-09-10)


### Bug Fixes

* use the right path for helm package ([2abb826](https://github.com/mogenius/renovate-operator/commit/2abb826c21636c3c074f306fe28331123241d7c1))

## [1.0.4](https://github.com/mogenius/renovate-operator/compare/1.0.3...1.0.4) (2025-09-10)


### Bug Fixes

* export go variables for controller-gen install ([a457308](https://github.com/mogenius/renovate-operator/commit/a4573080f6c5256247a270f43b11f56e1d5fac18))

## [1.0.3](https://github.com/mogenius/renovate-operator/compare/1.0.2...1.0.3) (2025-09-10)


### Bug Fixes

* setting gopath to install controller-gen ([7b8dd76](https://github.com/mogenius/renovate-operator/commit/7b8dd766c555b1324d10e944dafd599a40f331a7))

## [1.0.2](https://github.com/mogenius/renovate-operator/compare/1.0.1...1.0.2) (2025-09-10)


### Bug Fixes

* issues within the release action & helm chart ([50f2041](https://github.com/mogenius/renovate-operator/commit/50f2041fb8550307a5f21e38ca3b5d61a0ddf9a8))

## [1.0.1](https://github.com/mogenius/renovate-operator/compare/1.0.0...1.0.1) (2025-09-10)


### Bug Fixes

* use the correct helm name in github action ([f5560a1](https://github.com/mogenius/renovate-operator/commit/f5560a1b0b672fd098f3b4c94e9f4c408ad7a35b))

# 1.0.0 (2025-09-10)


### Features

* adding initial draft for the renovate operator ([db66ecc](https://github.com/mogenius/renovate-operator/commit/db66ecc996173f60e3c10044645926c77f8f8048))
