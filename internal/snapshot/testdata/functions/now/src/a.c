int pick(int a, int b) {
	if (a > 1 && a < 5 || b) {
		goto out;
	} else if (b) {
		a++;
	} else {
		a--;
	}
	for (int i = 0; i < a; i++) {
		while (b) {
			b = a ? 0 : 1;
		}
	}
	switch (a) {
	case 1:
		return 1;
	default:
		break;
	}
out:
	return 0;
}
