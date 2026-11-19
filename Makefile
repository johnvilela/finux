.PHONY: dev prod docs

# The version of a production build: the tag of the commit, or the last
# tag with the distance to it. A release sets VERSION to its tag.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# The development build uses its own database in the project root.
DEV_DB := $(CURDIR)/finux-dev.db

# Build the development CLI, then add sample data to its database. The
# database keeps its data between builds. 'make dev NEW_DB=1' deletes it
# first; make does not accept '--new-db' as an argument of a target.
dev:
	go build -gcflags="all=-N -l" -ldflags="-X 'main.devDatabase=$(DEV_DB)'" -o bin/finux-dev ./src
	./bin/finux-dev seed $(if $(NEW_DB),--new-db)

prod:
	go build -trimpath -ldflags="-s -w -X 'main.version=$(VERSION)'" -o bin/finux ./src

# Write the generated documentation for AI agents again: the reference of
# the MCP tools and the copies of the references in each skill.
docs:
	FINUX_UPDATE_DOCS=1 go test -count=1 -run 'TestAgentToolReferenceIsCurrent|TestAgentSkillReferencesAreCopies' ./src
