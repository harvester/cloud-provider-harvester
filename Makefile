TARGETS := $(shell ls scripts)

.dapper:
	@# 1. Run the script. It creates the file named '.dapper' which is validated by checksum
	./scripts/hack/install-dapper.sh
	@# 2. Verify the resulting binary
	./.dapper -v

$(TARGETS): .dapper
	./.dapper $@

trash: .dapper
	./.dapper -m bind trash

trash-keep: .dapper
	./.dapper -m bind trash -k

deps: trash

.DEFAULT_GOAL := ci

.PHONY: $(TARGETS)
