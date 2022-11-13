FROM golang as build
COPY . /src
RUN cd /src && go build -o deployer

FROM debian:bullseye
COPY --from=build /src/deployer /deployer
CMD ["/deployer"]
