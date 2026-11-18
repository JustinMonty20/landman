package parcelevidence

// minimalRotation is linear in ring length, including rings with repeated vertices.
func minimalRotation(vertices []string) []string {
	n := len(vertices)
	i, j, k := 0, 1, 0
	for i < n && j < n && k < n {
		a, b := vertices[(i+k)%n], vertices[(j+k)%n]
		if a == b {
			k++
			continue
		}
		if a > b {
			i += k + 1
			if i <= j {
				i = j + 1
			}
		} else {
			j += k + 1
			if j <= i {
				j = i + 1
			}
		}
		k = 0
	}
	start := min(i, j)
	out := make([]string, n)
	for offset := range out {
		out[offset] = vertices[(start+offset)%n]
	}
	return out
}
