G_DEFINE_TYPE(FuPlugin, fu_plugin, G_TYPE_OBJECT)

static gboolean
fu_plugin_load(FuPlugin *self, GError **error) G_GNUC_NON_NULL(1);

static gboolean
fu_plugin_load(FuPlugin *self, GError **error)
{
	g_autofree gchar *path = NULL;
	g_autoptr(GFile) file = NULL;
	if (path == NULL) {
		return FALSE;
	}
	g_assert_cmpint(self->len, ==, 1);
	return TRUE;
}
