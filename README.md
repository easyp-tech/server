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
wget https://raw.githubusercontent.com/easyp-tech/server/refs/heads/main/scripts/clone_repos.sh -O clone_repos.sh
```
```bash
chmod +x clone_repos.sh
```
```bash
./clone_repos.sh https://github.com/googleapis/googleapis.git https://github.com/bufbuild/protovalidate.git
```

#### Service Configuration

Create a service configuration file, replace easyp.tech with your domain:

```yaml
# address the service will be listening
listen:  127.0.0.1:8080

# domain the service will be accessible.
# Note: buf requires TLS, so this is a name your cert is assigned.
domain:  easyp.tech:8080

# Cache configuration
cache:
  # Cache type: none | local | artifactory
  # none means no cache, local means caching in the local directory
  # artifactory - surprise - means cache placed on the artifactory server
  # local/artifactory are configured with the corresponding sections below
  # Note: googleapis downloading from GitHub take about 40 sec,
  # so cache might be considered useful.
  type: none
  local:
    # directory where the cache will be placed.
    directory: ./.storage/.cache
  artifactory:
    token: some_artifactory_access_token
    user:  some_username
    url:   https://some.artifactory.url/with/read/write/access

# TLS config, required by buf.
tls:
  cert: cert.pem
  key:  key.pem
  #ca:   ca.pem

# Processing order:
# 1. check if the local mirror exists and use it if so
# 2. pass all the other to the GitHub proxy

# Locally mirrored git repos settings.
local:
  # Path to find the local git mirrors.
  # Mirrors must be os owner/repository subdir in this dir.
  # Local mirrors have the highest priority:
  # the server's response will be based on it if it is detected.  
  # Note: write access is required as the service will switch commits then needed.
  storage: ./.storage/gitlocal
  # Settings applied to the locally mirrored repos.
  # Mirrors not mentioned are processed as with empty config,
  # which is Ok for most cases.
  repo:
    - owner: googleapis
      name:  googleapis
      path:
        - google/type/
        - google/api/
        - google/rpc/
    - owner: bufbuild
      name:  protovalidate
      prefix:
        - proto/protovalidate/
    - owner: grpc-ecosystem
      name:  grpc-gateway
      path:
        - protoc-gen-openapiv2/

# Proxy config
# GitHub API only at the moment
proxy:
  # list of the repos will be proxied to BitBucket using API v1.
  bitbucket:
    # BitBucket API access token, generate your own.
    # You can go without token, but access rate will be really limited.
    # BitBucket repos are the second priority.
    - token: some_bitbucket_access_token
      user:  some_username
      url:   https://some.bitbucket.url/repo/is/accessible
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
    - token: some_bitbucket_access_token
      user: some_username
      url:   https://some.bitbucket.url/repo/is/accessible
      repo:
        owner: bufbuild
        name:  protovalidate
        # prefix is similar to path,
        # it is used to distinguish the files we really want to get.
        # but prefix will be cut off from the proxied file name.
        # it might be required for some repos, like bufbuild/protovalidate
        # Note: trailing slash is essential. Leading slash is prohibited.
        prefix:
          - proto/protovalidate/
    - token: some_bitbucket_access_token
      user: some_username
      url:   https://some.bitbucket.url/repo/is/accessible
      repo:
        owner: grpc-ecosystem
        name:  grpc-gateway
        path:
          - protoc-gen-openapiv2/

  # list of the repos will be proxied to GitHub.
  github:
    # GitHub API access token, generate your own.
    # You can go without token, but access rate will be really limited.
    # GitHub repos are the last priority.
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
    - token: some_github_token
      repo:
        owner: bufbuild
        name:  protovalidate
        # prefix is similar to path,
        # it is used to distinguish the files we really want to get.
        # but prefix will be cut off from the proxied file name.
        # it might be required for some repos, like bufbuild/protovalidate
        # Note: trailing slash is essential. Leading slash is prohibited.
        prefix:
          - proto/protovalidate/
    - token: some_github_token
      repo:
        owner: grpc-ecosystem
        name:  grpc-gateway
        path:
          - protoc-gen-openapiv2/
```

Note that config values can be set to env vars. Easyp will attempt to substitute them with the values from the current environment. If variable is unset, it will be replaced with an empty string. Example:

```yaml
# address the service will be listening
listen: ${EASYP_LISTEN}
```

#### `buf` configuration 

Essential part of `buf.yaml`:

```yaml
deps:
  - easyp.tech:8080/bufbuild/protovalidate
  - easyp.tech:8080/grpc-ecosystem/grpc-gateway
  - easyp.tech:8080/googleapis/googleapis
```
