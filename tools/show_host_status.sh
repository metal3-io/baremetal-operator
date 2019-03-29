#!/bin/bash

oc get baremetalhosts -o=custom-columns='NAME:.metadata.name,STATE:.status.provisioning.state,OP:.status.operationalStatus,ERROR:.status.errorMessage'
