# gRPEAKEC-go

gRPEAKEC-go is a collection of microservice utilities to allow the following:

- Easy to use rich error handling through gRPC interceptors, including native 
  errors.Is() and errors.As() support, sentinel error creation, tracing errors through
  services, and more.
  
- Dead simple service Manager that handles all the boilerplate of running a service,
  including resource setup and release, graceful shutdown, os signals and more.
  
- Testify Suite and testing methods for service Managers: write tests for your service
  endpoints and leave the rest to the suite.

## Getting Started
For full documentation:
[read the docs](https://peake100.github.io/gRPEAKEC-go/).

For library development guide, 
[read the docs](https://illuscio-dev.github.io/islelib-go/).


### Prerequisites

Golang 1.5+ (end-users), Python 3.6+ (developers)

Familiarity with [gRPC in Go](https://grpc.io/docs/languages/go/basics/)

## Authors

* **Billy Peake** - *Initial work*
