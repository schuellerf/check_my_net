# Check My Net
Creates a small overview of your favorite hosts to see if they are up and running

Example output:
```
Ping: Apr 30 17:20:56
dummy DNS (1.1.1.1)                6.267ms Apr 30 17:20:56.102 (1.1.1.1 - 0s ago)
dummy Router (192.168.0.1)         6.267ms Apr 30 17:20:56.102 (192.168.0.1 - 0s ago)
```

This will be updated periodically and can be customizable with a json file with your hosts

# Usage
Install Golang: https://golang.org/  
then run:
```
go get github.com/schuellerf/check_my_net 
go build github.com/schuellerf/check_my_net 
```
then in your user directory you should find a go/bin folder with the check_my_net binary
