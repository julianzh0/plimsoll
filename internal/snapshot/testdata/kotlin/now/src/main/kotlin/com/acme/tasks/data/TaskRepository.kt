package com.acme.tasks.data

import com.acme.tasks.legacy.Clock
import com.acme.tasks.model.*

class TaskRepository(private val clock: Clock) {
    fun save(tasks: List<Task>) = tasks.count { clock.due(it) >= 0 }
}
