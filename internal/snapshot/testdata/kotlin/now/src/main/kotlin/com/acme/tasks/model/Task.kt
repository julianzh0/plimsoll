package com.acme.tasks.model

data class Task(val title: String) {
    companion object {
        val none = Task("")
    }
}
