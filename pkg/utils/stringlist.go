package utils

func StringInList(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}

func FilterStringFromList(list []string, strToFilter string) (newList []string) {
	for _, item := range list {
		if item != strToFilter {
			newList = append(newList, item)
		}
	}
	return
}
