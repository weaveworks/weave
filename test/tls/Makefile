SERVERS = 192.168.48.11 192.168.48.12
SERVER_CAS  = $(SERVERS:=.ca.pem)
SERVER_CSRS = $(SERVERS:=.server.csr)
SERVER_PEMS = $(SERVERS:=.server.pem)
CLIENT_PEMS = $(SERVERS:=.client.pem)

.DEFAULT: all
.PHONY: all clean $(SERVERS)

SHELL=/bin/bash

all: ca-key.pem server-key.pem client-key.pem $(SERVER_CAS) $(SERVER_CSRS) $(SERVER_PEMS) client.csr $(CLIENT_PEMS)

ca-key.pem server-key.pem client-key.pem:
	openssl genrsa -out $@ 1024

$(SERVER_CAS): ca-key.pem
	openssl req -subj "/CN=$(@:.ca.pem=)" -new -x509 -days 3650 -key ca-key.pem -sha256 -out $@

$(SERVER_CSRS): server-key.pem
	openssl req -subj "/CN=$(@:.server.csr=)" -new -key server-key.pem -out $@

$(SERVER_PEMS): %.server.pem: %.server.csr ca-key.pem %.ca.pem
	openssl x509 -req -days 3650 -in $(@:.pem=.csr) \
		-CA $(@:.server.pem=.ca.pem) -CAkey ca-key.pem -CAcreateserial -out $@

client.csr: client-key.pem
	openssl req -subj '/CN=client' -new -key client-key.pem -out $@

$(CLIENT_PEMS): %.client.pem: client.csr ca-key.pem %.ca.pem
	openssl x509 -req -days 3650 -in client.csr \
		-CA $(@:.client.pem=.ca.pem) -CAkey ca-key.pem -CAcreateserial \
		-extfile <(echo "extendedKeyUsage = clientAuth") -out $@

clean:
	rm -f *.pem *.csr *.srl
