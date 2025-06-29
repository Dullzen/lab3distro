# Makefile para dist084 (clienteadmin y matchmaker)

all: docker-admin-matchmaker

# Build clienteadmin
build-clienteadmin:
    docker build -t clienteadmin ../clienteadmin

# Build matchmaker
build-matchmaker:
    docker build -t matchmaker ../matchmaker

# Run both clienteadmin and matchmaker
docker-admin-matchmaker: build-clienteadmin build-matchmaker
    -docker rm -f clienteadmin || true
    -docker rm -f matchmaker || true
    # Run matchmaker first (exposing the port)
    docker run -d --rm -p 50051:50051 --name matchmaker matchmaker
    # Run clienteadmin with interactive mode
    docker run --rm -it --name clienteadmin \
        -e MATCHMAKER_ADDRESS=localhost:50051 \
        clienteadmin

# Clean up
clean:
    -docker rm -f clienteadmin || true
    -docker rm -f matchmaker || true
    docker rmi -f clienteadmin matchmaker