import os
import sys

import yaml

CURRENT_DIR = os.path.dirname(os.path.realpath(__file__))
ANNOTATOR_PATH = os.path.join(CURRENT_DIR, "config", "overlays", "namespaced", "roles-ns-annotator.yaml")
MANAGER_PATCH_PATH = os.path.join(CURRENT_DIR, "config", "overlays", "namespaced", "namespaced-manager-patch.yaml")


def update_annotator_file(namespaces_to_set):
    # Load the YAML file
    with open(ANNOTATOR_PATH, 'r') as file:
        doc = yaml.safe_load(file)

    doc['values'] = [ns.strip() for ns in namespaces_to_set.split(',')]

    with open(ANNOTATOR_PATH, 'w') as file:
        yaml.dump(doc, file, default_flow_style=False)

def set_watch_namespace_env_var(namespaces_to_set):
    with open(MANAGER_PATCH_PATH, 'r') as file:
        doc = yaml.safe_load(file)
    
    # Dicts are always shallow copies and our patch is limited to one
    # one container and one environement value
    containers_spec = doc["spec"]["template"]["spec"]["containers"]
    containers_spec[0]["env"][0]["value"] = namespaces_to_set

    with open(MANAGER_PATCH_PATH, 'w') as file:
        yaml.dump(doc, file, default_flow_style=False)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: {} \"namespace1,namespace2\"")
        sys.exit(1)

    namespaces_to_set = sys.argv[1]
    
    update_annotator_file(namespaces_to_set)
    set_watch_namespace_env_var(namespaces_to_set)
