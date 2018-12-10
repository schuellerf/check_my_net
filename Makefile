all: bin/check_my_net
clean:
	rm bin/check_my_net
	rm -rf pkg/*
settings:
	@echo "We need root permissions to enable pinging"
	sysctl net.ipv4.ping_group_range|grep -vP "1[ \t]+0" || ( sudo sysctl -w net.ipv4.ping_group_range="0   2147483647" ; echo 'net.ipv4.ping_group_range = 0 2147483647' |sudo tee -a /etc/sysctl.conf )
src/github.com/aeden/traceroute/traceroute.go:
	go get github.com/aeden/traceroute
bin/check_my_net: settings Makefile src/github.com/schuellerf/check_my_net/check_my_net.go src/github.com/aeden/traceroute/traceroute.go
	go install github.com/schuellerf/check_my_net
	
