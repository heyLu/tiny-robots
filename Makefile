GOPATH=$(PWD)/.go
NAMESPACE=github.com/heyLu/tiny-robots
WORKSPACE=$(GOPATH)/src/$(NAMESPACE)

tiny-robots: $(WORKSPACE)
	cd $(WORKSPACE) && go install $(NAMESPACE)
	@cp $(GOPATH)/bin/$@ $(PWD)

$(WORKSPACE):
	mkdir -p $$(dirname $(WORKSPACE))
	ln -s $(PWD) $(WORKSPACE)
