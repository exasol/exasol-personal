<div align="center">

<picture>
  <source srcset="static/Exasol_Logo_2025_Bright.svg" media="(prefers-color-scheme: dark)">
  <img src="static/Exasol_Logo_2025_Dark.svg" alt="Exasol Logo" width="300">
</picture>

# Exasol Personal

**The Analytics Database for Agentic AI — Free for Personal Use**

*Deploy and explore Exasol Database on your own computer or in your cloud account.*

[![User documentation](https://img.shields.io/badge/user%20docs-latest-blue)](https://exasol.github.io/exasol-personal/latest/)
[![Exasol documentation](https://img.shields.io/badge/database%20docs-exasol.com-blue)](https://docs.exasol.com/db/latest/home.htm)
[![Community](https://img.shields.io/badge/community-exasol-green)](https://community.exasol.com)
[![Downloads](https://img.shields.io/badge/downloads-exasol.com-orange)](https://downloads.exasol.com/exasol-personal)

[Changelog](CHANGELOG.md)

</div>

## About Exasol Personal

Exasol Personal gives individual users a complete Exasol database for development, exploration,
analytics, and AI-assisted workflows. The Exasol Launcher is a scriptable command-line application
that installs and manages the database locally or on supported cloud infrastructure.

The database combines Exasol's in-memory, massively parallel analytics engine with the SQL,
integration, AI, and extensibility capabilities of Exasol Database. Exasol Personal is free for
personal use and does not impose an artificial data-size limit.

## Get started

The [latest user documentation](https://exasol.github.io/exasol-personal/latest/) contains the
current system requirements, installation instructions, supported deployment targets, tutorials,
command guidance, and troubleshooting information. Use its version selector when working with an
earlier release.

Download the launcher from the
[Exasol Download Portal](https://downloads.exasol.com/exasol-personal), or follow the
[installation guide](https://exasol.github.io/exasol-personal/latest/getting-started/).

AI coding agents can use the published
[Exasol agent skills](https://github.com/exasol-labs/exasol-agent-skills) to install Exasol Personal,
connect to it, load data, and write Exasol SQL.

## Development and contributions

This repository contains the Exasol Launcher, its built-in deployment presets, tests, and user
documentation sources. To build or contribute:

- Read the [contribution guidelines](CONTRIBUTING.md).
- Set up the project with the [development guide](doc/development.md).
- Follow the [testing guide](tests/README.md).
- Review the high-level [architecture](doc/architecture.md) and
  [project-specific best practices](doc/best_practices.md).

## License

The Exasol Launcher source code is licensed under the [MIT License](LICENSE). Exasol Database is
proprietary software provided by Exasol AG and is free for personal use. Installing the database
means accepting the
[Exasol Personal End User License Agreement](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement).

For help and discussion, visit the [Exasol Community](https://community.exasol.com) and use the
`exasol-personal` tag.
