# Testing Strategy

This document defines the testing goals and boundaries for Exasol Personal. Test structure, tooling, and CI behavior should follow from these goals.

## Goals

The test strategy optimizes for the following outcomes, in this order:

1. **Prove externally visible product behavior.** Users should be able to rely on the behavior described by the product specifications.
2. **Keep the project agile while internals evolve.** Internal refactoring should not require broad rewrites of product-level tests when user-visible behavior is unchanged.
3. **Give developers fast feedback.** Normal pull requests should receive useful test feedback within roughly 15 minutes and should not require cloud provisioning.
4. **Provide confidence in real deployments.** Supported deployment targets must be validated with live end-to-end tests where simulation is not sufficient.
5. **Keep contributors autonomous.** External contributors should be able to implement and test Exasol Personal itself without access to proprietary Exasol Database sources or Exasol-managed credentials.

Source-code coverage is a supporting signal only. Coverage of product requirements and scenarios matters more than line coverage.

## Testing Boundary

The test responsibility of this repository follows the implementation responsibility of Exasol Personal.

Everything implemented by Exasol Personal should be tested here. This includes, for example:

- CLI behavior and output contracts;
- deployment orchestration and lifecycle management;
- configuration and presets;
- provider integrations;
- local deployment behavior;
- Exasol Personal features for enabling or managing capabilities such as SLCs or AdminUI.

The deployed Exasol Database environment is part of the overall user experience, but its full correctness is outside the scope of this repository.

Exasol Personal tests should perform enough smoke validation to prove that Exasol Personal completed its job successfully. Examples include verifying that the deployed database and expected adjunct services are reachable and basically operational, or that a capability enabled by Exasol Personal can perform a minimal smoke operation.

They should not attempt comprehensive validation of Exasol Database, SQL semantics, UDF behavior, C4, COS, AdminUI, or other deployed components. Such testing belongs to a separate, deployment-agnostic system test suite that can be pointed at an existing deployment regardless of how that deployment was created.

A useful rule is:

> If the behavior is implemented by Exasol Personal, its primary regression test belongs here. If the behavior establishes the correctness of a component merely deployed by Exasol Personal, only the integration smoke check belongs here.

## Product Specifications and Test Evidence

OpenSpec is the intended source of truth for externally visible product behavior.

Product-facing OpenSpec scenarios should have automated end-to-end test evidence. A pull request that adds or changes such a scenario should normally add or update the corresponding end-to-end coverage.

The long-term goal is complete coverage of normative, product-facing specification scenarios. Specifications that describe internal constraints rather than product-observable behavior may be classified and verified differently.

Go tests are valuable developer evidence, but they do not replace end-to-end evidence for product specifications. Product Management and QA should be able to reason primarily about specification scenario coverage.

## Test Forms

### Go Tests

Go tests exercise implementation code directly. They are primarily for developers: fast feedback, regression detection, and support for good design.

Prefer real lightweight dependencies and test environments where practical, such as temporary files, directories, environment variables, and local test servers. Mock only where it improves determinism, safety, or speed.

Because implementation details are expected to evolve, implementation-level coverage may be deliberately lighter in highly volatile areas. Stable contracts should receive stronger and more durable coverage.

### End-to-End Tests

End-to-end tests receive a prebuilt `exasol` binary and exercise the product through supported external interfaces. They should not depend on Go implementation details or inspect private internal state as a substitute for observable product behavior.

Whether an end-to-end test needs a live deployment does not change its type. A fast CLI scenario that needs no infrastructure and a full deployment lifecycle scenario are both end-to-end tests; they differ only in cost and required environment.

End-to-end scenarios should be written from the user's perspective and, where applicable, correspond to OpenSpec scenarios.

### Specialized Suites

Some risks deserve distinct suites because they require special setup or intentionally unusual conditions:

- **Chaos/recovery tests** cover externally induced failures such as interrupted operations or unavailable dependencies.
- **Upgrade tests** cover compatibility and migration scenarios between versions.
- Other specialized suites, such as performance or security testing, can be introduced when concrete requirements justify them.

Expected command failures such as invalid input remain part of normal functional coverage rather than chaos testing.

## Organizing End-to-End Tests

The test suite should be easy to navigate from the product model and CLI structure, but it should not force commands to be tested in isolation.

Exasol Personal is stateful orchestration software. Many meaningful behaviors can only be observed after establishing a particular deployment state. It is therefore valid for one scenario to execute several commands to create its preconditions and verify its result.

Two complementary forms are useful:

- **Capability scenarios** focus on a product capability or specification scenario. They may invoke other commands to establish the required state.
- **User journeys and lifecycle scenarios** intentionally combine several capabilities and are especially useful when a live deployment can be reused efficiently.

Some overlap between these forms is desirable. Capability scenarios provide focused specification evidence; journeys prove that capabilities compose into a usable product.

Avoid accidental order dependencies between independently runnable tests. Intentional state progression inside one scenario or explicitly shared deployment context is acceptable.

## Stable Contracts

End-to-end tests should prefer stable, user-visible contracts over implementation artifacts.

For example, a successful deployment should normally be verified using command results and subsequent public commands such as `status`, `info`, or `connect`, rather than by inspecting internal state files. Internal files may change as the architecture evolves and should not become accidental public contracts through tests.

A major internal refactor should require few end-to-end changes unless the external product contract itself changes. If product-level tests are difficult to write without exposing implementation details, that is a useful signal to reconsider the architecture or contract.

## Pull Request Validation

Normal pull request validation should run all suitable fast, deterministic, unprivileged Go and end-to-end tests and should aim to complete within roughly 15 minutes.

Normal pull requests should not provision cloud infrastructure.

Tests that require live deployments remain available on demand and are part of release preparation. This balance may change as local deployment testing becomes cheaper and available on more CI platforms.

Internal and external contributions have the same quality expectations. Exasol-managed credentials and privileged infrastructure remain maintainer-controlled; contributors may use their own credentials and environments when they want to run live tests themselves.

## Provider and Platform Coverage

Every supported cloud preset should receive live end-to-end coverage.

There is no requirement to test every cloud provider from every launcher host platform. Host platform and cloud provider are largely independent, so the test matrix should combine them efficiently to obtain useful coverage without creating a full Cartesian product.

Local deployments are different because the host platform is the deployment platform. For release validation, local deployment behavior should therefore be tested on every supported host platform.

Generic deployment scenarios should be reusable across providers and platforms wherever the advertised product behavior is the same. Target-specific scenarios should cover genuine differences rather than duplicate generic behavior.

## Live Deployment Efficiency

Live deployments are expensive and should be created as rarely as practical.

A deployment may be reused across multiple compatible scenarios or across a longer user journey when this does not hide failures or make tests depend accidentally on execution order. Tests that intentionally destroy or corrupt a deployment may require isolated setup.

The objective is to minimize provisioning while still producing clear, trustworthy evidence of product behavior.

## Coverage and Quality Signals

There is no fixed source-code coverage target. Line or branch coverage may be used as a rough development signal, but it is not the primary measure of test quality.

The more important questions are:

- Which product requirements and OpenSpec scenarios have automated end-to-end evidence?
- Are the supported deployment targets represented by live validation?
- Can the internal architecture change without broad rewrites of product-level tests?
- Does the normal pull request suite provide fast, useful feedback?

These measures align testing with the actual product contract while preserving development agility.
