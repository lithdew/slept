# sleepy.

This is a repository where I will be breaking down existing libraries and protocols and things I've been reading into lessons and production-ready code for robust, high-performance p2p networking in Go.

## `sleepytcp`

It's interesting that there are three types of HTTP clients provided by [valyala/fasthttp](https://github.com/valyala/fasthttp): _Client_, _HostClient_, and _PipelineClient_.

A _Client_ manages a list of _HostClient_'s, and recycles them should they have no connections that are pending to be established/are already established every 10 seconds.

A _PipelineClient_ acts very much like a _Client_, though attempts to batch/blast/load balance as many requests as possible over multiple established connections to a particular server (aka request pipelining).

One problem with _PipelineClient_'s is that the latency of the server responding to its requests should be predictable and as small as possible. Otherwise, it will exhibit head-of-line blocking where stuck requests would cause other requests that were written after said request on the same connection would be stuck as well.

On the other hand, _Client_ and _HostClient_ does not exhibit this problem. The reason why is because at any moment in time, each connection may at most be handling one request/response at once. Should there be no available connections for a request to be made, _HostClient_ will spin up a new connection to then serve said request.

The behavior of _Client_ and _HostClient_ takes into consideration that one might be sending/receiving messages from multiple peers at once, and that new connections should be established to serve several concurrent requests/responses to particular choices of peers at once.

Hence, it is recommended to stave away from networking akin to the structure of _PipelineClient_ as head-of-line blocking _SHOULD_ be expected when networking amongst peers in a public network.

To deal with head-of-line blocking with TCP under a high-latency network, the safest solution is to use multiple parallel TCP connections connected to a single peer.

## `sleepyudp`

This is currently a work in progress, though the goal is to build a robust, high-performance, reliable UDP protocol on top of [reliable.io](https://gafferongames.com/post/reliable_ordered_messages/) for p2p networking.

As a reference, I have been reading code from:

1. The original reference reliable.io C implementation: [networkprotocol/reliable.io](https://github.com/networkprotocol/reliable.io)
2. An unoptimized reliable.io Go implementation, which appears to be a direct translation of the original reference C implementation: [jakecoffman/rely](https://github.com/jakecoffman/rely)
3. A realtime multiplayer reliable UDP networking library in Go using gafferongames' articles as a reference: [obsilp/rmnp](https://github.com/obsilp/rmnp) 