import os
import re
import sys

def update_rbac_markers(directory, namespace):
    rbac_pattern = re.compile(r'(\+kubebuilder:rbac:.*?namespace=")(.*?)(".*)')
    
    # If the namespace is an empty string, it should look like `namespace=""`
    new_namespace = namespace if namespace else ""
    
    for root, _, files in os.walk(directory):
        for file in files:
            file_path = os.path.join(root, file)
            if not file.endswith(".go"):
                continue
            
            with open(file_path, 'r') as f:
                content = f.read()
            
            new_content, count = rbac_pattern.subn(rf'\1{new_namespace}\3', content)
            
            if count > 0:
                with open(file_path, 'w') as f:
                    f.write(new_content)
                print(f"Updated {count} RBAC markers in {file_path}")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python update_kubebuilder_rbac.py <directory> <namespace>")
        sys.exit(1)
    
    directory = sys.argv[1]
    namespace = sys.argv[2]
    
    update_rbac_markers(directory, namespace)
