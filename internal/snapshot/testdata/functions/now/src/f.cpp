void count(QList<QString> l)
{
  int n = 0;
  Q_FOREACH (const QString &s, l) {
    if (s.isEmpty()) {
      n++;
    }
  }
}
