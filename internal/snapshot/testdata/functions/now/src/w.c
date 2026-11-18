_Must_inspect_result_ int
open_session(_In_ const char* name, _In_reads_(count) const int* values,
    _Out_opt_ int* result)
{
	if (name) {
		return 1;
	}
	return 0;
}

int count_streams(struct list *l)
{
	struct stream *s;
	int n = 0;
	LIST_FOREACH(s, l, link)
		if (s->on)
			n++;
	return n;
}
