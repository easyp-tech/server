# easyp-server

<!-- TOC -->
* [easyp-server](#easyp-server)
* [TODO:](#todo)
  * [Features](#features)
  * [Installation](#installation)
    * [Steps](#steps)
      * [Repository Setup](#repository-setup)
      * [Service Configuration](#service-configuration)
      * [`buf` configuration](#buf-configuration-)
<!-- TOC -->

# TODO:

* Elaborate on the roadmap

A backward-compatible server for working with the CLI tool [buf](https://github.com/bufbuild/buf).

## Features

* Support for the package manager with `buf mod update`

## Installation

### Steps

#### Repository Setup

```bash
wget https://github.com/easyp-tech/server/blob/main/scripts/clone_repos.sh -O clone_repos.sh

chmod +x clone_repos.sh

./clone_repos.sh https://github.com/googleapis/googleapis.git https://github.com/bufbuild/protovalidate.git
```

#### Service Configuration

Create a service configuration file, replace easyp.tech with your domain:

```yaml
# address the service will be listening
listen:  127.0.0.1:8080

# domain the service will be accessible.
# Note: buf requires TLS, so this is a name your cert isassigned.
domain:  easyp.tech:8080

# directory where the cache will be placed.
# Note: googleapis downloading take about 40 sec,
# so cache might be considered useful.
# Empty/unset cache means no caching.
cache: ./.storage/.cache

# TLS config, required by buf.
tls:
  cert: cert.pem
  key:  key.pem
  #ca:   ca.pem

# Processing order:
# 1. check if the local mirror exists and use it if so
# 2. pass all the other to the GitHub proxy

# Locally mirrored git repos settings.
# Mirrors exists but not mentioned here will be ignored.
local:
  # Path to find the local git mirrors.
  # Mirrors must be os owner/repository subdir in this dir.
  # Note: write access is required as the service will switch commits then needed.
  storage: ./.storage/gitlocal
  # Settings applied to the locally mirrored repos.
  # Mirrors not mentioned are processed as with empty config,
  # which is Ok for most cases.
  repo:
    # Repository details
    - owner: googleapis
      name:  googleapis
      # googleapis is huge.
      # Fortunately we do not need to get all the files to make it works.
      # So path is a list of prefixes for the files really required.
      # Note: trailing slash is essential. Leading slash is prohibited.
      path:
        - google/type/
        - google/api/
        - google/rpc/
    - owner: bufbuild
      name:  protovalidate
      # prefix is similar to path,
      # it is used to distinguish the files we really want to get.
      # but prefix will be cut off from the proxied file name.
      # it might be required for some repos, like bufbuild/protovalidate
      # Note: trailing slash is essential. Leading slash is prohibited.
      prefix:
        - proto/protovalidate/
    - owner: grpc-ecosystem
      name:  grpc-gateway
      path:
        - protoc-gen-openapiv2/

# Proxy config
# GitHub API only at the moment
proxy:
  # list of the repos will be proxied to GitHub.
  github:
    # GitHub API access token, generate your own.
    # You can go without token, but access rate will be really limited.
    # This token is used for the repos without specific token defined,
    # and also for the repos not directly mentioned here.
    token: some_github_token
    repo:
      # Repo-specific GitHub API access token.
      - token: some_github_token
        # Repository details
        repo:
          owner: googleapis
          name:  googleapis
          # googleapis is huge.
          # Fortunately we do not need to get all the files to make it works.
          # So path is a list of prefixes for the files really required.
          # Note: trailing slash is essential. Leading slash is prohibited.
          path:
            - google/type/
            - google/api/
            - google/rpc/
      - repo:
          owner: bufbuild
          name:  protovalidate
          # prefix is similar to path,
          # it is used to distinguish the files we really want to get.
          # but prefix will be cut off from the proxied file name.
          # it might be required for some repos, like bufbuild/protovalidate
          # Note: trailing slash is essential. Leading slash is prohibited.
          prefix:
            - proto/protovalidate/
      # Any repo not mentioned here directly will be proxied as is.
      #- repo:
      #    owner: grpc-ecosystem
      #    name:  grpc-gateway
      #    path:
      #      - protoc-gen-openapiv2/
```

#### `buf` configuration 

Essential part of `buf.yaml`:

```yaml
deps:
  - easyp.tech:8080/bufbuild/protovalidate
  - easyp.tech:8080/grpc-ecosystem/grpc-gateway
  - easyp.tech:8080/googleapis/googleapis
```