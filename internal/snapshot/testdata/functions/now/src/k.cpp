MY_API_EXPORT
MY_ALWAYS_INLINE int pick(int a) {
	if (a) {
		return 1;
	}
	return 0;
}

static MY_CONSTEXPR int keep(int a) {
	return a;
}

int clean(int a) {
	for (int i = 0; i < a; i++) {
		a--;
	}
	return a;
}
