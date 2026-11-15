import 'package:flutter_test/flutter_test.dart';
import 'package:notes/src/store.dart';

void main() {
  test('starts empty', () {
    expect(Store().notes, isEmpty);
  });
}
