# Local peer discovery script 

The simple peerfinder (peerfinder.py) script scans the nodes registered with the node directory (found under servers/) and returns the closest peers by latency.
To run this, you will need `fping` and some Python modules. On Debian/Ubuntu they can be installed with:

apt install fping python3 python3-yaml

