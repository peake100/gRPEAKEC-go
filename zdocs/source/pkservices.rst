pkservices
==========

package pkservices offers a lightweight framework for managing the lifetime of multiple
services and their resource dependencies (db connector, etc).

Quickstart
----------

To implement and run a simple gRPC service, do the following:



Service Interfaces
------------------

pkservices defines three interfaces for declaring services: ``Service``, ``ServiceGrpc``
and ``ServiceGeneric``.

These interfaces are designed for the quick declaration of services, which can then
be handed off to the ``pkservices.Manager`` type for running. The end result is
having to write very little code for the boilerplate of managing the lifetime of the
service.

.. note::

  ``Service`` acts as a base for more specific service types. A service must implement
  one of the more specific types (like ``ServiceGrpc``), and not just ``Service`` for
  the manager to run it.

The base ``Service`` interface looks like this:

