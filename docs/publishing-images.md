Publishing Images
=================

Images for changes merged into main are automatically built through
the [metal3-io org on
quay.io](https://quay.io/repository/metal3-io/baremetal-operator). It
is also easy to set up your own builds to test images from branches in
your development fork.

1. Fork `metal3-io/baremetal-operator` on GitHub.
2. Set up your account on [quay.io](https://quay.io).
3. Link your repository from step 1 to quay.io by following the
   instructions to "Create New Repository" from
   <https://quay.io/repository/>

   1. Enter the quay.io repository name. It is good practice to use
      the same name as the github repo.
   2. Make the repository public.
   3. Select "Link to a GitHub Repository Push" option at the bottom
      of the screen.
   4. Click "Create Public Repository"
   5. When prompted, authenticate to GitHub to allow quay.io to see
      your repositories.
   6. Select the GitHub organization, usually your personal repo org.
   7. Select the repository from the list. If you have a lot of repos,
      it may help to type part of the name into the filter box to
      reduce the length of the list.
   8. Click "Continue"
   9. Configure your trigger. Selecting "Trigger for all branches and
      tags" allows you to build images from branches automatically,
      and is good for a developer configuraiton.
   10. Click "Continue"
   11. Enter the Dockerfile name: `build/Dockerfile`
   12. Enter the "context" where Docker will run the build. This is
       usually `/`, to indicate the root of your repository.
   13. Click "Continue"
   14. Skip setting up a robot account and click "Continue" again.
   15. Click "Continue" again (yes, 3 times in a row).
   16. At this point, quay.io adds an ssh key to your repository.
   17. Click the "Return to ..." link.

4. Test a build

   1. Click "Start New Build"
   2. Click "Run Trigger Now" in the modal popup
   3. Select "main" from the list of branches.
   4. Click "Start Build"
   5. At this point you may have to refresh your browser to see the
      build because the UI seems to cache pretty aggressively.

5. Create a dev deployment file that uses your image instead of the
   one from the metal3-io organization.

   1. Copy `deploy/operator.yaml` to `deploy/dev-operator.yaml`.
   2. Edit `deploy/dev-operator.yaml` and change the `image` setting
      so it points to your account on quay.io. Builds from main will
      have the default "latest" tag, but in order to test from a
      branch you will need to modify the image name to include the
      branch at the end, like this:

          quay.io/dhellmann/baremetal-operator:update-operator-deployment

6. Launch the deployment by applying the new file.

   1. Make sure you have run the [setup steps](dev-setup.md) to set up
      the service account, role, and mapping.
   2. Apply the new deployment:

       kubectl apply -f deploy/dev-operator.yaml

To monitor the operator, use `kubectl get pods` to find the pod name for
the deployment (it will start with `baremetal-operator`) and then use
`kubectl log -f $podname` to see the console log output.
