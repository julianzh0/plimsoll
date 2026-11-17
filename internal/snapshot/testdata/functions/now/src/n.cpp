class Foo {
 public:
  int get() const { return 1; }
  ~Foo() {}
  Foo operator+(const Foo& o) { return o; }
};
template <typename T> T& ns::Baz<T>::qux() { static T t; return t; }
