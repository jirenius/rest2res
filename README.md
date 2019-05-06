<p align="center"><a href="https://resgate.io" target="_blank" rel="noopener noreferrer"><img width="100" src="https://resgate.io/img/resgate-logo.png" alt="Resgate logo"></a></p>


<h2 align="center"><b>REST to RES service</b><br/>Synchronize Your Clients</h2>
</p>
<p align="center">
<a href="http://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"></a>
<a href="http://goreportcard.com/report/jirenius/rest2res"><img src="http://goreportcard.com/badge/github.com/jirenius/rest2res" alt="Report Card"></a>
</p>

---

*REST to RES* (*rest2res* for short) is a service for [Resgate](https://resgate.io) and [NATS Server](https://nats.io) that can turn old JSON based legacy REST APIs into live APIs.

Visit [Resgate.io](https://resgate.io) for more information on Resgate.

## Polling madness
Do you have lots of clients polling every X second from your REST API, to keep their views updated? This is madness :scream:!

By placing *rest2res/resgate* in between as a cache, you can reduce thousands of polling clients to a single *rest2res* service doing the polling on their behalf.

The clients will instead fetch the same data from *Resgate*, whose cache is efficiently updated by *rest2res* whenever there is a modification. No changes has to be made to the legacy REST API, nor the clients (except perhaps changing which URL to poll from).

Your clients can later be improved to use [ResClient](https://www.npmjs.com/package/resclient), which uses WebSocket, to reliably get the updates as soon as they are detected, without having to do any polling at all. This will majorly decrease the bandwidth required serving your clients.

 ## Quickstart

### Download Resgate/NATS Server
This service uses *Resgate* and *NATS Server*. You can just download one of the pre-built binaries:
* [Download](https://nats.io/download/nats-io/gnatsd/) and run NATS Server
* [Download](https://github.com/jirenius/resgate/releases/latest) and run Resgate

> **Tip**
>
> If you run Resgate with:  `resgate --apiencoding=jsonflat`  
> the web resource JSON served by Resgate will look the same as the legacy REST API for nested JSON structures, without href meta data.

### Build rest2res

First make sure you have [installed Go](https://golang.org/doc/install). Then you can download and compile the service:

```text
git clone github.com/jirenius/rest2res
cd rest2res
go build
```

### Try it out

Run it with one of the example configs, such as for *worldclockapi.com*:
```text
rest2res --config=examples/clock.config.json
```

Access the data through Resgate:

```text
http://localhost:8080/api/clock/utc/now
```

Or get live data using [ResClient](https://resgate.io/docs/writing-clients/resclient/):

```javascript
import ResClient from 'resclient';

let client = new ResClient('ws://localhost:8080');
client.get('clock.utc.now').then(model => {
   console.log(model.currentDateTime);
   model.on('change', () => {
      console.log(model.currentDateTime);
   });
});
```

## Usage
```text
rest2res [options]
```

| Option | Description | Default value
|---|---|---
| `-n, --nats <url>` | NATS Server URL | `nats://127.0.0.1:4222`
| `-c, --config <file>` | Configuration file (required) |
| `-h, --help` | Show usage message |


## Configuration

Configuration is a JSON encoded file. It is a json object containing the following available settings:

**natsUrl** *(string)*  
NATS Server URL. Must be a valid URI using `nats://` as schema.  
*Default:* `"nats://127.0.0.1:4222"`

**serviceName** *(string)*  
Name of the service, used as the first part of all resource IDs.  
*Default:* `rest2res`

**externalAccess** *(boolean)*  
Flag telling if access requests are handled by another service. If false, rest2res will handle access requests by granting full access to all endpoints.  
*Default:* `false`

**endpoints** *(array of endpoints)*  
List of endpoints handled by rest2res. See below for [endpoint configuration](#endpoint).  
*Default:* `[]`

> **Tip**
>
> A new configuration file with default settings can be created by using the `--config` option, specifying a file path that does not yet exist.
>
> ```text
> rest2res --config myconfig.json
> ```

### Endpoint

An endpoint is a REST endpoint to be mapped to RES. It is a json object with the following available settings:

**url** *(string)*  
URL to the legacy REST API endpoint. May contain `${tags}` as placeholders for URL parameters.  
*Example:* `"http://worldclockapi.com/api/json/${timezone}/now"`

**refreshTime** *(number)*  
The duration in milliseconds between each poll to the legacy endpoint.  
*Default:* `5000`

**refreshCount** *(number)*  
Number of times *rest2res* should poll a resource before asking Resgate(s) if any client is still interested in the data.  
A high number may cause unnecessary polling, while a low number may cause additional traffic between rest2res and Resgate.  
*Default:* `6`

**timeout** *(number)*  
Time in milliseconds before client requests should timeout, in case the endpoint is slow to respond. If `0`, or not set, the default request timeout of Resgate is used.  
*Example:* `5000`

**type** *(string)*  
Type of data for the legacy endpoint. The setting tells *rest2res* if it should expect the legacy endpoint to return an *object* or an *array*.

* `model` - used for a JSON objects
* `collection` - used for a JSON arrays

*Example:* `"model"`

**pattern** *(string)*  
The resource ID pattern for the endpoint resource.  
The pattern often follows a similar structure as the URL path, but is dot-separated instead of slash-separated. A part starting with a dollar sign is considered a placeholder (eg. `$tags`). The pattern must contain placeholders matching the placeholder names used in the endpoint *url* setting.  
*Example:* `"$timezone.now"`

**resources** *(array of resources)*  
List of nested resources (objects and array) within the endpoint root data. See below for [resource configuration](#resource).  
*Example:* `[{ "type":"model", "path":"foo" }]`

### Resource

A resource, in this context, is an object or array nested within the endpoint data. It is called *resource* as it will be mapped to its own [RES resource](https://resgate.io/docs/writing-services/02basic-concepts/#resources) with a unique *resource ID*. The configuration is a JSON object with following available settings:

**type** *(string)*  
Type of data for the sub-resource.

* `model` - used for a JSON objects
* `collection` - used for a JSON arrays

*Example:* `"model"`

**pattern** *(string)*  
The resource ID pattern for the sub-resource.  
May be omitted. It omitted, the pattern will be the same as the parent resource pattern suffixed by the *path* setting.  
*Example:* `"station.$stationId.transfers"`

**path** *(string)*  
The path to the resource relative to the parent resource.  
If the parent resource is an object/model, the *path* is either:

* name of the parent property key (eg. `"foo"`)
* a placeholder starting with `$`, acting as a wildcard for all parent property keys (eg. `"$property"`).  

If the parent resource is an array/collection, the *path* is a placeholder starting with `$` (eg. `$userId`). The placeholder represents either:

* index in the parent array
* model id, in case `idProp` is set (see below).

**idProp** *(string)*  
ID property in an object, used to identify it within a parent array/collection.  
Only valid for *object* types.  
*Example:* `"_id"`

**resources** *(array of resources)*  
List of nested [resources](#resource) (objects and array) within the sub-resource.  
*Example:* `[{ "type":"model", "path":"bar" }]`

> **Tip**
>
> Does configuring an endpoint seem complicated?  
> Check out the example configs in the [`/examples`](examples/) folder.

## Caveats

The data fetched by *rest2res* will be shared through Resgate's cache with all clients requesting the same data. This means that the legacy REST API endpoints must be completely open for *rest2res* to access, as it will never access a legacy REST endpoint on behalf of a specific client.

**Authorization**  
If client authorization is needed, this can be handled by creating a separate authentication and authorization service. Read more about [access control on Resgate.io](https://resgate.io/docs/writing-services/07access-control/).

**User specific data**  
While it is possible to have user specific resources, *rest2res* does not support having a single endpoint URL returning different data depending on which user accesses it.  
But seriously, no proper REST resource should return different results on the same URL.


## Contribution

If you find any issues with the service, feel free to report them. This project caters to some specific needs, and may yet lack features to make it viable for all use cases.

If you lack a feature, feel free to create an issue for it to be discussed. If you like coding in Go, maybe you can make a pull request with the solution concluded from the discussion.