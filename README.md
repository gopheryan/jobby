## Jobby

A job management system

### Testing
Run tests with `make test`

Start the server: `make start-server`

Send a 'start job' request: `make start-job`


### Certificate Creation
All required x509 certificates to run the server and client have been checked into `testdata/certs`

Makefile recipes to regenerate certs are included but should not be needed for basic testing. Id you'd like to generate a new client cert with a different username, you can do so with: `CLIENT_NAME=<name> make gen-client-cert`

Server and client certs are automatically signed using the `ca.crt`, `ca.key` pair found in `testdata/certs/ca`