all: bin/check_my_net
clean:
	rm bin/check_my_net
	rm -rf pkg/*
settings:
	@echo "We need root permissions to enable pinging"
	sysctl net.ipv4.ping_group_range|grep -vP "1[ \t]+0" || sudo sysctl -w net.ipv4.ping_group_range="0   2147483647"
bin/check_my_net: settings Makefile src/github.com/schuellerf/check_my_net/check_my_net.go
	go install github.com/schuellerf/check_my_net
	
