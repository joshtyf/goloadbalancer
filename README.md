# Load Balancer

This is a simple project that simulates a load balancer. I created it out of fun and thought that load balancers would be interesting to implement.

## Quickstart

Use the following command to start the project:

```
go run .
```

It will start the load balancer on port 8080. It will also create mock servers from port 8082 to 8089. Any requests to the load balancer will be forwarded to one of the mock servers based on the algorithm configured.

## Possible Improvements

- Add more algorithms
- Implement ListenerGroups
- Read configuration from a file
- Use Docker to create the mock servers
- Allow load balancer to forward requests to different servers based on the path
