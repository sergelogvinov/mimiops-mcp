# Contributing to Mimiops MCP Server

## Reporting Issues

We use GitHub issues to track bugs and feature requests.
Before creating a new issue, please search the existing issues to see if your problem has already been reported.

## Pull Requests

All PRs require a single commit.

Having one commit in a Pull Request is very important for several reasons:
* A single commit per PR keeps the git history clean and readable.
  It helps reviewers and future developers understand the change as one atomic unit of work, instead of sifting through many intermediate or redundant commits.
* One commit is easier to cherry-pick into another branch or to track in changelogs.
* Squashing into one meaningful commit ensures the final PR only contains what matters.

### Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/). Each commit message must follow the format:

```text
<type>(<optional scope>): <description>

[optional body]
```

Common types include:

* `feat` -- a new feature
* `fix` -- a bug fix
* `refactor` -- code changes that neither fix a bug nor add a feature
* `chore` -- other changes that don't modify source or test files

## Sign Your Commits

All commits must include a `Signed-off-by` trailer.
This certifies that you wrote the patch or otherwise have the right to pass it on as an open-source patch under the [Developer Certificate of Origin](https://developercertificate.org/).

```shell
git commit --signoff -m "feat: add new feature"
```

## Conformance

To verify conformance status, run `make conformance`.
This runs a series of tests on the working tree and is required to pass before a contribution is accepted.

## Development

```sh
make lint        # golangci-lint
make unit        # unit tests
make test        # lint + unit
make licenses    # license audit (forbidden/restricted/unknown)
make conformance # conformance tests
```

Use `make help` for a full target list.

## Code of Conduct

We expect all contributors to adhere to the [Code of Conduct](CODE_OF_CONDUCT.md).
