# Contributing to Exasol Personal

Thank you for your interest in contributing! We welcome contributions from the community.

## How to Contribute

1. **Fork the repository** and clone your fork
2. **Set up your development environment** - see [Development Guide](doc/development.md)
3. **Create a branch** for your changes: `git checkout -b feature/your-feature-name`
4. **Make your changes** following our [coding guidelines](doc/best_practices.md)
5. **Test your changes** - see [Development Guide](doc/development.md#testing)
6. **Submit a pull request**

## Pull Request Guidelines

- **Keep PRs focused** - one feature or fix per PR
- **Write clear descriptions** - explain what and why, not just how
- **Use semantic commit messages** - follow [Conventional Commits](https://www.conventionalcommits.org/) format:
  - `feat:` new features
  - `fix:` bug fixes
  - `docs:` documentation changes
  - `test:` test additions or changes
  - `refactor:` code refactoring
  - `chore:` maintenance tasks
- **Add tests** for new features or bug fixes
- **Update documentation** if you're changing functionality
- **Run `task fmt` and `task lint`** before committing
- **Ensure all tests pass** - run `task all` to check
- **Reference related issues** using `#issue-number`

## Code Review Process

- All submissions require review before merging
- Be responsive to feedback and questions
- Address review comments promptly
- Maintainers will merge approved PRs

## Reporting Issues

- Use GitHub Issues for bugs and feature requests
- Search existing issues first to avoid duplicates
- For bugs: provide clear reproduction steps and environment details
- For features: explain the use case and expected behavior

## Community Guidelines

- Be respectful and professional
- Welcome newcomers and help others
- Provide constructive feedback

## Licensing

This repository is licensed under the [MIT License](./LICENSE). By submitting a pull request, you agree that your contributions are licensed under the same terms.

Note that the Exasol database deployed by this tool is proprietary software under its own [EULA](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement). Contributions to this repository do not affect the database license.

## Questions?

- Check the [documentation](doc/)
- Ask in GitHub Discussions
- Use tag `exasol-personal` on [Exasol Community](https://community.exasol.com)
