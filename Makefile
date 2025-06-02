.PHONY: test
test: echo
	@go test -v ./...

# requires c compiler
.PHONY: test-race
test-race: echo
	CGO_ENABLED=1 go test -race ./...

.PHONY: echo
echo: testdata/testprograms/echo

testdata/testprograms/echo: cmd/echo/main.go
	go build -o testdata/testprograms/echo cmd/echo/main.go

# 'experimental_allow_proto3_optional' allows compatibility with older versions of protoc (like mine)
#  since proto3 did not always support field presence
.PHONY: protos
protos:
	protoc --experimental_allow_proto3_optional --go_out=jobmanagerpb --go_opt=paths=source_relative --go-grpc_out=jobmanagerpb --go-grpc_opt=paths=source_relative jobby.proto


# Starts the server using the certs provided in the testdata/certs directory
# Shut down with ctrl+c
.PHONY: start-server
start-server:
	{ cd testdata/certs; go run ../../cmd/server/main.go; }

# Starts a short job using 'jobcli' and attaches to its output
# Assumes you have the server running already
.PHONY: jobcli-echo-and-attach
jobcli-echo-and-attach: echo
	{ cd testdata/certs; \
	  go run ../../cmd/jobcli start ../../testdata/testprograms/echo echo 15 | tail -c +13 | xargs go run ../../cmd/jobcli attach; \
	}

# Demonstrates sending a request via grpcurl
.PHONY: start-job
start-job: echo
	grpcurl -cacert testdata/certs/ca/ca.crt -cert testdata/certs/client/${CLIENT_NAME}/client.crt -key testdata/certs/client/${CLIENT_NAME}/client.key \
		 -d '{"command": "../../testdata/testprograms/echo", "args": ["echo", "10"]}' \
		localhost:8443 jobby.JobManager.StartJob

# Quick and dirty recipes for generating CA, server, and client certs and keys.
# This project is taking a lot of time and I hope this is "good enough"

CERTS_DIR:=testdata/certs
CA_CERT_PATH:=${CERTS_DIR}/ca
SERVER_CERT_PATH:=${CERTS_DIR}/server
CLIENT_NAME ?= ryan
CLIENT_CERT_PATH:=${CERTS_DIR}/client/${CLIENT_NAME}
.PHONY: gen-ca
gen-ca:
	mkdir -p ${CA_CERT_PATH}
	openssl ecparam -name prime256v1 -genkey -noout -out ${CA_CERT_PATH}/ca.key
	openssl req -x509 -new -nodes -key ${CA_CERT_PATH}/ca.key -sha256 -days 365 \
	  -subj "/CN=JobbyRootCA" \
	  -out ${CA_CERT_PATH}/ca.crt


.PHONY: gen-server-cert
gen-server-cert:
	mkdir -p ${SERVER_CERT_PATH}
	openssl ecparam -name prime256v1 -genkey -noout -out ${SERVER_CERT_PATH}/server.key
	openssl req -new -key ${SERVER_CERT_PATH}/server.key -out ${SERVER_CERT_PATH}/server.csr \
	-subj "/CN=localhost"
	openssl x509 -req -in ${SERVER_CERT_PATH}/server.csr -CA ${CA_CERT_PATH}/ca.crt -CAkey ${CA_CERT_PATH}/ca.key \
		-CAcreateserial -out ${SERVER_CERT_PATH}/server.crt -days 365 -sha256 \
		-extfile ${CERTS_DIR}/server-ext.cnf \
		-extensions v3_req \

# Invoke as: CLIENT_NAME=<name> make gen-client-cert
# to generate a signed client cert and key for a different user
.PHONY: gen-client-cert
gen-client-cert:
	mkdir -p ${CLIENT_CERT_PATH}
	openssl ecparam -name prime256v1 -genkey -noout -out ${CLIENT_CERT_PATH}/client.key
	openssl req -new -key ${CLIENT_CERT_PATH}/client.key -out ${CLIENT_CERT_PATH}/client.csr \
	-subj "/CN=${CLIENT_NAME}"
	openssl x509 -req -in ${CLIENT_CERT_PATH}/client.csr -CA ${CA_CERT_PATH}/ca.crt -CAkey ${CA_CERT_PATH}/ca.key \
		-CAcreateserial -out ${CLIENT_CERT_PATH}/client.crt -days 365 -sha256

