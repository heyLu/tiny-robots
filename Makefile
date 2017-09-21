GOPATH=$(PWD)/.go
NAMESPACE=github.com/heyLu/tiny-robots
WORKSPACE=$(GOPATH)/src/$(NAMESPACE)

VERSION=0.0.1

IMAGE_PREFIX=
IMAGE_NAME=$(IMAGE_PREFIX)tiny-robots:$(VERSION)

tiny-robots: $(WORKSPACE)
	cd $(WORKSPACE) && go install $(NAMESPACE)
	@cp $(GOPATH)/bin/$@ $(PWD)

tiny-robots-static: $(WORKSPACE)
	cd $(WORKSPACE) && CGO_ENABLED=0 go build -o $@ -v $(NAMESPACE)

docker: tiny-robots-static
	docker build -t $(IMAGE_NAME) .

$(WORKSPACE):
	mkdir -p $$(dirname $(WORKSPACE))
	ln -s $(PWD) $(WORKSPACE)
