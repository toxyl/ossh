package main

func MaxOfInts(val int, compare ...int) int {
	if len(compare) <= 0 {
		return val
	}
	c := val
	for _, cmp := range compare {
		if cmp > c {
			c = cmp
		}
	}
	return c
}

func SumOfInts(values ...int) int {
	c := 0
	for _, v := range values {
		c += v
	}
	return c
}

func SumOfUints(values ...uint) uint {
	c := uint(0)
	for _, v := range values {
		c += v
	}
	return c
}

func SumOfFloats(values ...float64) float64 {
	c := 0.0
	for _, v := range values {
		c += v
	}
	return c
}
