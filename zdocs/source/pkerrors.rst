pkerrs
======

The ``pkerrs`` package offers a system for defining, returning, and handling rich
errors between gRPC services and clients.

The Error Generator
-------------------

The core type of pkerrs is the ``ErrorGenerator`` type. We will use the generator to
declare our error types, generate new instances of those types, and create gRPC server
and client interceptors that serialize and deserialize these errors.

We can set up a new generator as follows:


