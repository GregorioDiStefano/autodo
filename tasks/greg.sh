#!/bin/bash
echo $(echo -n $JSON | base64 -d)
exit 1
