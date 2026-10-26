Each implementation task below maps to exactly one code-only commit that passes
the full check suite. Group 5 is validation and produces no commit.

## 1. Endpoint declaration schema

- [x] 1.1 Add the `Endpoint` type with `port`, `scheme` and `path`, accept
  `endpoints` beside `container` on a sidecar definition, resolve a catalog
  entry as a whole so enabling saves the declarations with it, carry the
  declarations through cloning and parsing, and validate alongside the container
  that the named port exists, that the scheme is present and well formed, and
  that a path begins with `/`. Verify `task tests-unit` covers a valid
  declaration, an unknown port name, a missing scheme, a malformed path, and a
  saved document that declares no endpoints.

## 2. Status reports complete URLs

- [x] 2.1 Give `SidecarEndpoint` a `url` field, carry the saved declarations
  from status through sidecar inspection into endpoint observation, match each
  observed binding to its declaration by host port, build the URL from the
  observed host address, and print it on the endpoint line in place of the host
  and port. Verify `task tests-unit` covers the forwarding and direct runtime
  paths, an IPv6 loopback address, a published port with no declaration, and a
  declaration on an internal port.

## 3. Catalog declarations

- [x] 3.1 Declare `http` and `/mcp` on the MCP entry's `mcp` port in the
  embedded catalog, keep the saved declarations when a live test rewrites a
  container definition, and assert the reported `url` in the MCP live tests.
  Verify `task tests-launcher` passes.

## 4. Documentation

- [x] 4.1 Document the `endpoints` declaration and the reported URL in the user
  sidecar page and the catalog section of the developer sidecar page, replace
  the paragraph telling readers to prefix `http://` and append `/mcp`, add `url`
  to the documented JSON status keys, and extend the existing unreleased
  changelog entry for sidecars. Verify `task docs-check` passes.

## 5. Validation

- [x] 5.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch.
