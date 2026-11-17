class Panel : public QWidget {
  Q_OBJECT
  Q_PROPERTY(int size READ size WRITE setSize)
public:
  int size() const;
public slots:
  void refresh();
Q_SIGNALS:
  void changed();
};

int Panel::size() const {
  Q_D(const Panel);
  int n = 0;
  foreach (const QString &tag, d->tags) {
    if (!tag.isEmpty()) {
      n++;
    }
  }
  emit changed();
  return n;
}
