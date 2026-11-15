package com.acme.tasks.data

import com.acme.tasks.model.Task

class TestTaskDao(private val saved: MutableList<Task>) : AutoCloseable {
    fun insert(tasks: List<Task>) = tasks.forEach {
        it.also(saved::add)
    }

    override fun close() = saved.clear()
}
