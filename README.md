# sniff

Sniff is a comprehensive database of code reviews about code smells. This repository provides the essentials to replicate its construction.

## Prerequisites

- [Go](https://go.dev/doc/install)


## Usage

<!-- ### Crawler -->

To get started with Sniff, follow these steps:

1. Create .env:
```bash
cp .env.example .env
```

2. Paste your [GitHub token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) in .env.

3. Build the application:
```bash
go build src/main.go
```

4. Run the application:
```bash
./main
```