#include <stdexcept>
#include <vector>

auto chain = [](int a) { return a && a > 1 || a < 0; };

int Store::route(std::vector<int>& xs, int a) {
	auto f = [&](int x) { if (x) { return 1; } return 0; };
	try {
		for (auto v : xs) {
			if (v > a) { a = f(v); }
		}
	} catch (const std::exception& e) {
		a = 0;
	}
	return a > 0 ? a : -a;
}
