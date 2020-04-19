[![](https://github.com/doitintl/cluster-scheduler/workflows/Docker%20Image%20CI/badge.svg)](https://github.com/doitintl/cluster-scheduler/actions?query=workflow%3A"Docker+Image+CI") [![Docker Pulls](https://img.shields.io/docker/pulls/doitintl/cluster-scheduler.svg?style=popout)](https://hub.docker.com/r/doitintl/cluster-scheduler) [![](https://images.microbadger.com/badges/image/doitintl/cluster-scheduler.svg)](https://microbadger.com/images/doitintl/cluster-scheduler "Get your own image badge on microbadger.com")

# cluster-scheduler

The `cluster-scheduler` helps you to reduce cloud cost for managed Kubernetes clusters (GKE and EKS), by stopping and restarting Kubernetes clusters on schedule.

## Google Cloud

### Required Permissions

`container.clusters.update`


## Build Project

### Docker

The `cluster-scheduler` uses Docker both as a CI tool and for releasing final `cluster-scheduler` Docker image (`scratch` with updated `ca-credentials` package).

### Makefile

The `cluster-scheduler` `Makefile` is used for task automation only: compile, lint, test and etc.

### Continuous Integration

GitHub action `Docker CI` is used for `cluster-scheduler` CI.

#### Required GitHub secrets

Please specify the following GitHub secrets:

1. `DOCKER_USERNAME` - Docker Registry username
2. `DOCKER_PASSWORD` - Docker Registry password or token
3. `DOCKER_REGISTRY` - _optional_; Docker Registry name, default to `docker.io`
4. `DOCKER_REPOSITORY` - _optional_; Docker image repository name, default to `$GITHUB_REPOSITORY` (i.e. `user/repo`)
