#!/bin/bash
sleep 60
echo $(echo -n $JSON | base64 -d)
