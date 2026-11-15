import 'package:flutter/material.dart';
import 'store.dart';

class NotesApp extends StatelessWidget {
  const NotesApp({super.key});
  @override
  Widget build(BuildContext context) => Text(Store().title);
}
