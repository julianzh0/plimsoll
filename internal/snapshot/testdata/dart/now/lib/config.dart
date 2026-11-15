import 'dart:io';

final String apiUrl = Platform.environment['NOTES_API'] ?? '';
String setting(String key) => Platform.environment[key] ?? '';
