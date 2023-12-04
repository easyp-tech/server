# easyp-server

TODO:

* Provide an example of a proxy setup
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
server:
  external:
    domain: "easyp.tech"
    host: "0.0.0.0"
    port:
      connect: 8080
      metric: 8081
storage:
  root: "/cache"
```

#### Docker Compose Setup

```yaml
version: "3.9"

services:

  easyp:
    image: easyp/server:v0.1.0
    restart: always
    command: [
      "-cfg=/config.yml",
    ]
    volumes:
      - "./config.yml:/config.yml"
      - "./easyp_volume:/cache/"
```
