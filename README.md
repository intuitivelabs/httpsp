# httpsp

This go modules provides HTTP message "stream"/stateful parsing functions.
(work in progress)

It supports parsing partial HTTP messages received on streams: if the
parsing functions detect an incomplete message, they will signal
this through the return values and parsing can be latter resumed
from the point where it stopped.

