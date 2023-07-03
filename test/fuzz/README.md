# Fuzzing

 Fuzzing or fuzz testing is an automated software testing technique that
 involves providing invalid, unexpected, or random data as inputs to a
 computer program. A fuzzing target is defined for running the tests.
 These targets are defined in the test/fuzz directory and run using
 the instructions given below:

- Run below command to run the fuzzing target function from the root directory.
   This command running a fuzzing.sh script inside the test/fuzz directory which
   runs all the required target functions one after the other. Currently they are
   run for 120 seconds i.e. 2 minutes. We can increase the time to make it run
   according to our need.

    ```bash
    make fuzz
    ```

- Execution: Here is an example for execution of some test outputs:

    ```bash
    before &{Scenario: Host:{TypeMeta:{Kind: APIVersion:}
    ....
    ErrorCount:0}} Expected:false}
    actual false
    ```

    Here first line contains the byte array provided by libfuzzer to the
    function we then typecast it to our struct of IPPool using JSON.
    Unmarshal and then run the function. It keeps on running
    until it finds a bug.