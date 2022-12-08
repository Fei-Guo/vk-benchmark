# Virtual Kubelet Benchmark

## How to build?
- Build binary: `make build`
- Build image: `make build-image`
- Push image


## How to run?

- Change the image location in `./config/setup/all_in_one.yaml`
- Do `kubectl apply -f ./config/setup/all_in_one.yaml`
